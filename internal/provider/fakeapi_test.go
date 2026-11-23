package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

// fakeAPI is an in-memory stand-in for the control plane. It decodes request
// bodies strictly (like the real server) and completes queued tasks on the
// second poll, so the provider's wait loops are exercised.
type fakeAPI struct {
	t        *testing.T
	mu       sync.Mutex
	seq      int
	tokens   map[string]*client.EnrollmentToken
	nodes    map[string]*client.Node
	vms      map[string]*client.VM
	targets  map[string]*client.BackupTarget
	policies map[string]*client.BackupPolicy
	tasks    map[string]*fakeTask
	settleTo map[string]string // VM id -> state it reaches on its next read
	server   *httptest.Server
}

type fakeTask struct {
	task  client.Task
	polls int
	apply func() // runs when the task succeeds
}

const fakeToken = "test-admin-token"

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{
		t: t, tokens: map[string]*client.EnrollmentToken{}, nodes: map[string]*client.Node{},
		vms: map[string]*client.VM{}, targets: map[string]*client.BackupTarget{},
		policies: map[string]*client.BackupPolicy{}, tasks: map[string]*fakeTask{}, settleTo: map[string]string{},
	}
	f.nodes["node-1"] = &client.Node{
		ID: "node-1", Name: "hv01", State: client.NodeReady, Online: true, Address: "10.0.0.11",
		Inventory: client.NodeInventory{Hostname: "hv01", CPUCores: 16, MemoryBytes: 64 << 30, Bridges: []string{"virbr0"}},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAPI) id(prefix string) string {
	f.seq++
	return fmt.Sprintf("%s-%d", prefix, f.seq)
}

func (f *fakeAPI) enqueue(typ, resID string, apply func()) *client.Task {
	ft := &fakeTask{task: client.Task{ID: f.id("task"), Type: typ, Status: client.TaskQueued, ResourceID: resID}, apply: apply}
	f.tasks[ft.task.ID] = ft
	t := ft.task
	return &t
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "message": msg})
}

func strictDecode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+fakeToken {
		apiError(w, 401, "unauthorized", "missing or invalid admin token")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	route := r.Method + " " + parts[0]
	if len(parts) > 1 {
		route += "/{id}"
	}
	if len(parts) > 2 {
		route += "/" + strings.Join(parts[2:], "/")
	}
	bad := func(err error) { apiError(w, 400, "invalid", "invalid request body: "+err.Error()) }

	switch {
	// --- tasks
	case route == "GET tasks/{id}":
		ft, ok := f.tasks[parts[1]]
		if !ok {
			apiError(w, 404, "not_found", "task not found")
			return
		}
		ft.polls++
		if ft.polls == 1 && !ft.task.Done() {
			ft.task.Status = client.TaskRunning
		} else if !ft.task.Done() {
			ft.task.Status = client.TaskSucceeded
			if ft.apply != nil {
				ft.apply()
			}
		}
		writeJSON(w, 200, ft.task)

	// --- enrollment tokens
	case route == "POST enrollment-tokens":
		var req struct {
			Description string `json:"description"`
			TTLMinutes  int    `json:"ttlMinutes"`
		}
		if err := strictDecode(r, &req); err != nil {
			bad(err)
			return
		}
		t := &client.EnrollmentToken{ID: f.id("tok"), Description: req.Description, ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}
		f.tokens[t.ID] = t
		writeJSON(w, 201, client.CreatedToken{Token: t, Value: "avt_secret_" + t.ID})
	case route == "GET enrollment-tokens":
		out := []*client.EnrollmentToken{}
		for _, t := range f.tokens {
			out = append(out, t)
		}
		writeJSON(w, 200, out)
	case route == "DELETE enrollment-tokens/{id}":
		if _, ok := f.tokens[parts[1]]; !ok {
			apiError(w, 404, "not_found", "token not found")
			return
		}
		delete(f.tokens, parts[1])
		w.WriteHeader(204)

	// --- nodes / bridges
	case route == "GET nodes":
		out := []*client.Node{}
		for _, n := range f.nodes {
			out = append(out, n)
		}
		writeJSON(w, 200, out)
	case route == "GET nodes/{id}":
		n, ok := f.nodes[parts[1]]
		if !ok {
			apiError(w, 404, "not_found", "node not found")
			return
		}
		writeJSON(w, 200, n)
	case route == "POST nodes/{id}/bridges":
		var req struct {
			Name      string `json:"name"`
			Mode      string `json:"mode,omitempty"`
			IPv4      string `json:"ipv4,omitempty"`
			DHCPStart string `json:"dhcpStart,omitempty"`
			DHCPEnd   string `json:"dhcpEnd,omitempty"`
		}
		if err := strictDecode(r, &req); err != nil {
			bad(err)
			return
		}
		n := f.nodes[parts[1]]
		writeJSON(w, 202, f.enqueue("bridge.create", n.ID, func() {
			n.Inventory.Bridges = append(n.Inventory.Bridges, req.Name)
			n.Inventory.BridgeNetworks = append(n.Inventory.BridgeNetworks, client.BridgeNetwork(req))
		}))
	case route == "DELETE nodes/{id}/bridges/"+lastPart(parts):
		n, name := f.nodes[parts[1]], lastPart(parts)
		if r.URL.Query().Get("confirm") != name {
			apiError(w, 400, "invalid", "confirm must equal the bridge name")
			return
		}
		writeJSON(w, 202, f.enqueue("bridge.delete", n.ID, func() {
			n.Inventory.Bridges = remove(n.Inventory.Bridges, name)
			kept := n.Inventory.BridgeNetworks[:0]
			for _, b := range n.Inventory.BridgeNetworks {
				if b.Name != name {
					kept = append(kept, b)
				}
			}
			n.Inventory.BridgeNetworks = kept
		}))

	// --- VMs
	case route == "POST vms":
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			ClusterID   string `json:"clusterId"`
			NodeID      string `json:"nodeId"`
			VCPUs       int    `json:"vcpus"`
			MemoryMiB   int    `json:"memoryMiB"`
			Firmware    string `json:"firmware"`
			CPUMode     string `json:"cpuMode"`
			BootISO     string `json:"bootIso"`
			Disks       []struct {
				SizeGiB     int    `json:"sizeGiB"`
				Pool        string `json:"pool"`
				SourceImage string `json:"sourceImage"`
			} `json:"disks"`
			NICs      []client.NICSpec  `json:"nics"`
			Tags      map[string]string `json:"tags"`
			Start     bool              `json:"start"`
			Autostart bool              `json:"autostart"`
		}
		if err := strictDecode(r, &req); err != nil {
			bad(err)
			return
		}
		for _, vm := range f.vms {
			if vm.Name == req.Name {
				apiError(w, 409, "conflict", fmt.Sprintf("a VM named %q already exists", req.Name))
				return
			}
		}
		spec := client.VMSpec{Name: req.Name, UUID: f.id("uuid"), VCPUs: req.VCPUs, MemoryMiB: req.MemoryMiB,
			Firmware: req.Firmware, CPUMode: req.CPUMode, BootISO: req.BootISO, NICs: req.NICs, Autostart: req.Autostart}
		for i, d := range req.Disks {
			pool := d.Pool
			if pool == "" {
				pool = "default"
			}
			spec.Disks = append(spec.Disks, client.DiskSpec{Device: fmt.Sprintf("vd%c", 'a'+i), SizeGiB: d.SizeGiB, Pool: pool, SourceImage: d.SourceImage})
		}
		if len(spec.NICs) == 0 {
			spec.NICs = []client.NICSpec{{Network: "default"}}
		}
		for i := range spec.NICs {
			if spec.NICs[i].MAC == "" {
				spec.NICs[i].MAC = fmt.Sprintf("52:54:00:00:00:%02x", f.seq%256)
			}
			if spec.NICs[i].Model == "" {
				spec.NICs[i].Model = "virtio"
			}
		}
		vm := &client.VM{ID: f.id("vm"), Name: req.Name, Description: req.Description, NodeID: "node-1",
			State: client.VMCreating, Spec: spec, Tags: req.Tags, Snapshots: []client.Snapshot{}}
		f.vms[vm.ID] = vm
		final := client.VMStopped
		if req.Start {
			final = client.VMRunning
		}
		// The create task is not returned; the VM settles on its next read.
		f.settleTo[vm.ID] = final
		writeJSON(w, 202, vm)
	case route == "GET vms":
		out := []*client.VM{}
		for _, vm := range f.vms {
			out = append(out, f.settle(vm))
		}
		writeJSON(w, 200, out)
	case route == "GET vms/{id}":
		vm, ok := f.vms[parts[1]]
		if !ok {
			apiError(w, 404, "not_found", "vm not found")
			return
		}
		writeJSON(w, 200, f.settle(vm))
	case route == "DELETE vms/{id}":
		vm, ok := f.vms[parts[1]]
		if !ok {
			apiError(w, 404, "not_found", "vm not found")
			return
		}
		if r.URL.Query().Get("confirm") != vm.Name {
			apiError(w, 400, "invalid", "confirmation required: pass confirm="+vm.Name)
			return
		}
		vm.State = client.VMDeleting
		writeJSON(w, 202, f.enqueue("vm.delete", vm.ID, func() { delete(f.vms, vm.ID) }))
	case strings.HasPrefix(route, "POST vms/{id}/actions/"):
		vm := f.vms[parts[1]]
		next := map[string]string{"start": client.VMRunning, "stop": client.VMStopped, "pause": client.VMPaused, "resume": client.VMRunning}[parts[3]]
		writeJSON(w, 202, f.enqueue("vm."+parts[3], vm.ID, func() { vm.State = next }))
	case route == "POST vms/{id}/snapshots":
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := strictDecode(r, &req); err != nil {
			bad(err)
			return
		}
		vm := f.vms[parts[1]]
		writeJSON(w, 202, f.enqueue("snapshot.create", vm.ID, func() {
			vm.Snapshots = append(vm.Snapshots, client.Snapshot{Name: req.Name, Description: req.Description, CreatedAt: time.Now()})
		}))
	case strings.HasPrefix(route, "DELETE vms/{id}/snapshots/"):
		vm, name := f.vms[parts[1]], parts[3]
		writeJSON(w, 202, f.enqueue("snapshot.delete", vm.ID, func() {
			kept := vm.Snapshots[:0]
			for _, s := range vm.Snapshots {
				if s.Name != name {
					kept = append(kept, s)
				}
			}
			vm.Snapshots = kept
		}))

	// --- backup targets and policies
	case route == "POST backup-targets":
		var req struct {
			Name        string   `json:"name"`
			Type        string   `json:"type"`
			Path        string   `json:"path"`
			NFSServer   string   `json:"nfsServer"`
			NFSExport   string   `json:"nfsExport"`
			ClusterIDs  []string `json:"clusterIds"`
			Description string   `json:"description"`
		}
		if err := strictDecode(r, &req); err != nil {
			bad(err)
			return
		}
		t := &client.BackupTarget{ID: f.id("bt"), Name: req.Name, Type: req.Type, Path: req.Path, NFSServer: req.NFSServer,
			NFSExport: req.NFSExport, ClusterIDs: req.ClusterIDs, Description: req.Description, CreatedAt: time.Now()}
		if t.ClusterIDs == nil {
			t.ClusterIDs = []string{}
		}
		f.targets[t.ID] = t
		writeJSON(w, 201, t)
	case route == "GET backup-targets":
		out := []*client.BackupTarget{}
		for _, t := range f.targets {
			out = append(out, t)
		}
		writeJSON(w, 200, out)
	case route == "DELETE backup-targets/{id}":
		for _, p := range f.policies {
			if p.TargetID == parts[1] {
				apiError(w, 409, "conflict", "target is used by policy "+p.Name)
				return
			}
		}
		delete(f.targets, parts[1])
		w.WriteHeader(204)
	case route == "POST backup-policies", route == "PUT backup-policies/{id}":
		var req client.PolicyRequest
		if err := strictDecode(r, &req); err != nil {
			bad(err)
			return
		}
		if _, ok := f.targets[req.TargetID]; !ok {
			apiError(w, 404, "not_found", "backup target not found")
			return
		}
		id, status := f.id("bp"), 201
		if r.Method == http.MethodPut {
			id, status = parts[1], 200
		}
		next := time.Now().Add(time.Hour)
		p := &client.BackupPolicy{ID: id, Name: req.Name, Enabled: req.Enabled, Schedule: req.Schedule, TargetID: req.TargetID,
			KeepLast: req.KeepLast, Selector: req.Selector, NextRunAt: &next}
		f.policies[id] = p
		writeJSON(w, status, p)
	case route == "GET backup-policies":
		out := []*client.BackupPolicy{}
		for _, p := range f.policies {
			out = append(out, p)
		}
		writeJSON(w, 200, out)
	case route == "DELETE backup-policies/{id}":
		delete(f.policies, parts[1])
		w.WriteHeader(204)

	default:
		f.t.Errorf("fake API: unhandled route %s %s", r.Method, r.URL.Path)
		apiError(w, 404, "not_found", "no route "+route)
	}
}

// settle finishes VM creation on the first read after POST, like the agent
// reporting the create task result.
func (f *fakeAPI) settle(vm *client.VM) *client.VM {
	if final, ok := f.settleTo[vm.ID]; ok {
		delete(f.settleTo, vm.ID)
		vm.State, vm.Defined = final, true
		for i := range vm.Spec.Disks {
			vm.Spec.Disks[i].Path = "/var/lib/libvirt/images/" + vm.Name + "-" + vm.Spec.Disks[i].Device + ".qcow2"
		}
	}
	return vm
}

func lastPart(p []string) string { return p[len(p)-1] }

func remove(in []string, s string) []string {
	out := []string{}
	for _, v := range in {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
