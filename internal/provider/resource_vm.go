package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*vmResource)(nil)
	_ resource.ResourceWithImportState = (*vmResource)(nil)
)

type vmResource struct{ base }

func newVMResource() resource.Resource { return &vmResource{} }

type vmModel struct {
	ID                   types.String   `tfsdk:"id"`
	Name                 types.String   `tfsdk:"name"`
	Description          types.String   `tfsdk:"description"`
	ClusterID            types.String   `tfsdk:"cluster_id"`
	NodeID               types.String   `tfsdk:"node_id"`
	VCPUs                types.Int64    `tfsdk:"vcpus"`
	MemoryMiB            types.Int64    `tfsdk:"memory_mib"`
	Firmware             types.String   `tfsdk:"firmware"`
	CPUMode              types.String   `tfsdk:"cpu_mode"`
	BootISO              types.String   `tfsdk:"boot_iso"`
	Disks                types.List     `tfsdk:"disks"`
	NICs                 types.List     `tfsdk:"nics"`
	Tags                 types.Map      `tfsdk:"tags"`
	Autostart            types.Bool     `tfsdk:"autostart"`
	PowerState           types.String   `tfsdk:"power_state"`
	DeleteDisksOnDestroy types.Bool     `tfsdk:"delete_disks_on_destroy"`
	State                types.String   `tfsdk:"state"`
	LastError            types.String   `tfsdk:"last_error"`
	UUID                 types.String   `tfsdk:"uuid"`
	Timeouts             timeouts.Value `tfsdk:"timeouts"`
}

type vmDiskModel struct {
	SizeGiB     types.Int64  `tfsdk:"size_gib"`
	Pool        types.String `tfsdk:"pool"`
	SourceImage types.String `tfsdk:"source_image"`
	Device      types.String `tfsdk:"device"`
	Path        types.String `tfsdk:"path"`
}

type vmNICModel struct {
	Network types.String `tfsdk:"network"`
	Bridge  types.String `tfsdk:"bridge"`
	Model   types.String `tfsdk:"model"`
	MAC     types.String `tfsdk:"mac"`
}

var vmDiskAttrTypes = map[string]attr.Type{
	"size_gib": types.Int64Type, "pool": types.StringType, "source_image": types.StringType,
	"device": types.StringType, "path": types.StringType,
}

var vmNICAttrTypes = map[string]attr.Type{
	"network": types.StringType, "bridge": types.StringType, "model": types.StringType, "mac": types.StringType,
}

// replaceOnLengthChange forces replacement when list elements are added or
// removed. Changes inside elements are handled by the nested attributes.
func replaceOnLengthChange() planmodifier.List {
	return listplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.ListRequest, resp *listplanmodifier.RequiresReplaceIfFuncResponse) {
		if req.PlanValue.IsUnknown() || req.StateValue.IsNull() {
			return
		}
		resp.RequiresReplace = len(req.PlanValue.Elements()) != len(req.StateValue.Elements())
	}, "Adding or removing elements replaces the VM.", "Adding or removing elements replaces the VM.")
}

func (r *vmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (r *vmResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepReplaceIfSet := []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplaceIfConfigured()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A KVM virtual machine. The scheduler places it on a node in `cluster_id` (or anywhere in the " +
			"fleet) unless `node_id` pins it. The API cannot resize or reconfigure an existing VM, so changes to the " +
			"hardware, disks, NICs or tags replace it. `power_state` is changed in place with lifecycle actions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name": schema.StringAttribute{
				Required: true, PlanModifiers: replace, Validators: []validator.String{nameValidator()},
				MarkdownDescription: "VM (libvirt domain) name, unique across the fleet.",
			},
			"description": schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"cluster_id": schema.StringAttribute{
				Optional: true, Computed: true, PlanModifiers: keepReplaceIfSet,
				MarkdownDescription: "Cluster to place the VM in. Empty lets the scheduler pick any cluster or standalone node.",
			},
			"node_id": schema.StringAttribute{
				Optional: true, Computed: true, PlanModifiers: keepReplaceIfSet,
				MarkdownDescription: "Pin the VM to a node. Computed from placement when not set.",
			},
			"vcpus": schema.Int64Attribute{
				Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"memory_mib": schema.Int64Attribute{
				Required: true, Validators: []validator.Int64{int64validator.AtLeast(64)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				MarkdownDescription: "Memory in MiB.",
			},
			"firmware": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("bios"), PlanModifiers: replace,
				Validators: []validator.String{stringvalidator.OneOf("bios", "uefi")}, MarkdownDescription: "`bios` (default) or `uefi`.",
			},
			"cpu_mode": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("host-model"), PlanModifiers: replace,
				Validators:          []validator.String{stringvalidator.OneOf("host-model", "host-passthrough")},
				MarkdownDescription: "`host-model` (default) or `host-passthrough` (less portable across CPUs).",
			},
			"boot_iso": schema.StringAttribute{
				Optional: true, PlanModifiers: replace, MarkdownDescription: "Absolute path of an ISO on the node to boot from.",
			},
			"disks": schema.ListNestedAttribute{
				Required: true, MarkdownDescription: "Disks in order (`vda`, `vdb`, ...). All disks must use the same pool.",
				Validators:    []validator.List{listvalidator.SizeBetween(1, 26)},
				PlanModifiers: []planmodifier.List{replaceOnLengthChange()},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"size_gib": schema.Int64Attribute{
						Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)},
						PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
					},
					"pool": schema.StringAttribute{
						Optional: true, Computed: true, Default: stringdefault.StaticString("default"), PlanModifiers: replace,
						MarkdownDescription: "Storage pool. Defaults to `default`.",
					},
					"source_image": schema.StringAttribute{
						Optional: true, PlanModifiers: replace,
						MarkdownDescription: "Absolute path of a backing image (template) on the node.",
					},
					"device": schema.StringAttribute{Computed: true, PlanModifiers: keep, MarkdownDescription: "Guest device, e.g. `vda`."},
					"path":   schema.StringAttribute{Computed: true, PlanModifiers: keep, MarkdownDescription: "Disk path on the node."},
				}},
			},
			"nics": schema.ListNestedAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Network interfaces. When omitted the VM gets one virtio NIC on the libvirt `default` network.",
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown(), replaceOnLengthChange()},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"network": schema.StringAttribute{Optional: true, PlanModifiers: replace, MarkdownDescription: "libvirt network name."},
					"bridge":  schema.StringAttribute{Optional: true, PlanModifiers: replace, MarkdownDescription: "Host bridge name."},
					"model": schema.StringAttribute{
						Optional: true, Computed: true, Default: stringdefault.StaticString("virtio"), PlanModifiers: replace,
					},
					"mac": schema.StringAttribute{
						Optional: true, Computed: true, PlanModifiers: keepReplaceIfSet,
						MarkdownDescription: "MAC address; generated when omitted.",
					},
				}},
			},
			"tags": schema.MapAttribute{
				ElementType: types.StringType, Optional: true,
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
				MarkdownDescription: "Tags, used for example by backup policy selectors.",
			},
			"autostart": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
				MarkdownDescription: "Start the domain when the host boots.",
			},
			"power_state": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString(client.VMRunning),
				Validators:          []validator.String{stringvalidator.OneOf(client.VMRunning, client.VMStopped, client.VMPaused)},
				MarkdownDescription: "Desired power state: `running` (default), `stopped` or `paused`. Changed in place.",
			},
			"delete_disks_on_destroy": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Delete the VM's disks when it is destroyed. Defaults to `true`; set `false` to keep " +
					"the disk files on the host. Backups are always kept.",
			},
			"state":      schema.StringAttribute{Computed: true, MarkdownDescription: "Current VM state reported by the control plane."},
			"last_error": schema.StringAttribute{Computed: true, MarkdownDescription: "Last task error, if any."},
			"uuid":       schema.StringAttribute{Computed: true, PlanModifiers: keep, MarkdownDescription: "libvirt domain UUID."},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Update: true, Delete: true}),
		},
	}
}

func (r *vmResource) buildCreate(ctx context.Context, m *vmModel, diags *diag.Diagnostics) client.CreateVMRequest {
	var disks []vmDiskModel
	diags.Append(m.Disks.ElementsAs(ctx, &disks, false)...)
	req := client.CreateVMRequest{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueString(),
		ClusterID:   m.ClusterID.ValueString(),
		NodeID:      m.NodeID.ValueString(),
		VCPUs:       int(m.VCPUs.ValueInt64()),
		MemoryMiB:   int(m.MemoryMiB.ValueInt64()),
		Firmware:    m.Firmware.ValueString(),
		CPUMode:     m.CPUMode.ValueString(),
		BootISO:     m.BootISO.ValueString(),
		Tags:        toStringMap(ctx, m.Tags, diags),
		Start:       m.PowerState.ValueString() != client.VMStopped,
		Autostart:   m.Autostart.ValueBool(),
	}
	for _, d := range disks {
		req.Disks = append(req.Disks, client.CreateDiskRequest{
			SizeGiB: int(d.SizeGiB.ValueInt64()), Pool: d.Pool.ValueString(), SourceImage: d.SourceImage.ValueString(),
		})
	}
	if !m.NICs.IsNull() && !m.NICs.IsUnknown() {
		var nics []vmNICModel
		diags.Append(m.NICs.ElementsAs(ctx, &nics, false)...)
		for _, n := range nics {
			req.NICs = append(req.NICs, client.NICSpec{
				Network: n.Network.ValueString(), Bridge: n.Bridge.ValueString(),
				Model: n.Model.ValueString(), MAC: n.MAC.ValueString(),
			})
		}
	}
	return req
}

// setVMState maps the API object onto the model, keeping optional values the
// user left out as null.
func setVMState(ctx context.Context, m *vmModel, vm *client.VM, diags *diag.Diagnostics) {
	m.ID = types.StringValue(vm.ID)
	m.Name = types.StringValue(vm.Name)
	m.Description = keepNullString(m.Description, vm.Description)
	m.ClusterID = types.StringValue(vm.ClusterID)
	m.NodeID = types.StringValue(vm.NodeID)
	m.VCPUs = types.Int64Value(int64(vm.Spec.VCPUs))
	m.MemoryMiB = types.Int64Value(int64(vm.Spec.MemoryMiB))
	m.Firmware = types.StringValue(orDefault(vm.Spec.Firmware, "bios"))
	m.CPUMode = types.StringValue(orDefault(vm.Spec.CPUMode, "host-model"))
	m.BootISO = keepNullString(m.BootISO, vm.Spec.BootISO)
	m.Autostart = types.BoolValue(vm.Spec.Autostart)
	m.State = types.StringValue(vm.State)
	m.LastError = types.StringValue(vm.LastError)
	m.UUID = types.StringValue(vm.Spec.UUID)
	switch vm.State {
	case client.VMRunning, client.VMStopped, client.VMPaused:
		m.PowerState = types.StringValue(vm.State)
	}
	if m.PowerState.IsNull() || m.PowerState.IsUnknown() {
		m.PowerState = types.StringValue(client.VMRunning)
	}
	if m.DeleteDisksOnDestroy.IsNull() || m.DeleteDisksOnDestroy.IsUnknown() {
		m.DeleteDisksOnDestroy = types.BoolValue(true)
	}

	var priorDisks []vmDiskModel
	if !m.Disks.IsNull() && !m.Disks.IsUnknown() {
		diags.Append(m.Disks.ElementsAs(ctx, &priorDisks, false)...)
	}
	disks := make([]vmDiskModel, 0, len(vm.Spec.Disks))
	for i, d := range vm.Spec.Disks {
		src := types.StringNull()
		if i < len(priorDisks) {
			src = priorDisks[i].SourceImage
		}
		disks = append(disks, vmDiskModel{
			SizeGiB:     types.Int64Value(int64(d.SizeGiB)),
			Pool:        types.StringValue(orDefault(d.Pool, "default")),
			SourceImage: keepNullString(src, d.SourceImage),
			Device:      types.StringValue(d.Device),
			Path:        types.StringValue(d.Path),
		})
	}
	m.Disks = objectList(ctx, vmDiskAttrTypes, disks, diags)

	var priorNICs []vmNICModel
	if !m.NICs.IsNull() && !m.NICs.IsUnknown() {
		diags.Append(m.NICs.ElementsAs(ctx, &priorNICs, false)...)
	}
	nics := make([]vmNICModel, 0, len(vm.Spec.NICs))
	for i, n := range vm.Spec.NICs {
		prior := vmNICModel{Network: types.StringNull(), Bridge: types.StringNull()}
		if i < len(priorNICs) {
			prior = priorNICs[i]
		}
		nics = append(nics, vmNICModel{
			Network: keepNullString(prior.Network, n.Network),
			Bridge:  keepNullString(prior.Bridge, n.Bridge),
			Model:   types.StringValue(orDefault(n.Model, "virtio")),
			MAC:     types.StringValue(n.MAC),
		})
	}
	m.NICs = objectList(ctx, vmNICAttrTypes, nics, diags)

	tags := stringMap(ctx, vm.Tags, diags)
	m.Tags = keepNullMap(m.Tags, tags)
}

// waitVMSettled waits until the VM leaves the transitional states.
func (r *vmResource) waitVMSettled(ctx context.Context, id string) (*client.VM, error) {
	var vm *client.VM
	err := r.client.Poll(ctx, "VM "+id, func(ctx context.Context) (bool, error) {
		v, err := r.client.GetVM(ctx, id)
		if err != nil {
			return false, err
		}
		vm = v
		switch v.State {
		case client.VMPending, client.VMCreating, client.VMRestoring:
			return false, nil
		case client.VMError:
			return false, fmt.Errorf("VM %s is in error state: %s", v.Name, v.LastError)
		}
		return true, nil
	})
	return vm, err
}

// runAction queues a lifecycle action and waits for it.
func (r *vmResource) runAction(ctx context.Context, id, action string) error {
	t, err := r.client.VMAction(ctx, id, action)
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	_, err = r.client.WaitTask(ctx, t.ID)
	return err
}

// ensurePowerState drives the VM from its current state to want.
func (r *vmResource) ensurePowerState(ctx context.Context, id, want string) (*client.VM, error) {
	for range 3 {
		vm, err := r.client.GetVM(ctx, id)
		if err != nil {
			return nil, err
		}
		if vm.State == want {
			return vm, nil
		}
		var action string
		switch {
		case want == client.VMStopped && (vm.State == client.VMRunning || vm.State == client.VMPaused):
			action = "stop"
		case want == client.VMRunning && vm.State == client.VMPaused:
			action = "resume"
		case want == client.VMPaused && vm.State == client.VMRunning:
			action = "pause"
		case vm.State == client.VMStopped || vm.State == client.VMUnknown:
			action = "start" // to running, or as the first step to paused
		default:
			return vm, fmt.Errorf("cannot move VM %s from %s to %s", vm.Name, vm.State, want)
		}
		if err := r.runAction(ctx, id, action); err != nil {
			return vm, err
		}
	}
	vm, err := r.client.GetVM(ctx, id)
	if err == nil && vm.State != want {
		err = fmt.Errorf("VM %s is %s, expected %s", vm.Name, vm.State, want)
	}
	return vm, err
}

func (r *vmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 30*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body := r.buildCreate(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	vm, err := r.client.CreateVM(ctx, body)
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating VM", err)
		return
	}
	want := plan.PowerState.ValueString()
	setVMState(ctx, &plan, vm, &resp.Diagnostics)
	plan.PowerState = types.StringValue(want)
	// Record the VM right away so a failed wait leaves it tainted, not orphaned.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if vm, err = r.waitVMSettled(ctx, vm.ID); err == nil {
		vm, err = r.ensurePowerState(ctx, vm.ID, want)
	}
	if vm != nil {
		setVMState(ctx, &plan, vm, &resp.Diagnostics)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if err != nil {
		apiErr(&resp.Diagnostics, "Waiting for VM", err)
	}
}

func (r *vmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vm, err := r.client.GetVM(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading VM", err)
		return
	}
	setVMState(ctx, &state, vm, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Update(ctx, 20*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	want := plan.PowerState.ValueString()
	vm, err := r.ensurePowerState(ctx, state.ID.ValueString(), want)
	if vm != nil {
		setVMState(ctx, &plan, vm, &resp.Diagnostics)
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Changing VM power state", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 20*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id := state.ID.ValueString()
	vm, err := r.client.GetVM(ctx, id)
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading VM", err)
		return
	}
	if vm.State != client.VMDeleting {
		task, err := r.client.DeleteVM(ctx, id, vm.Name, state.DeleteDisksOnDestroy.ValueBool())
		if client.IsNotFound(err) {
			return
		}
		if err != nil {
			apiErr(&resp.Diagnostics, "Deleting VM "+vm.Name, err)
			return
		}
		if task == nil {
			return // record removed immediately: creation had failed on the host
		}
		if _, err := r.client.WaitTask(ctx, task.ID); err != nil {
			apiErr(&resp.Diagnostics, "Deleting VM "+vm.Name, err)
			return
		}
	}
	err = r.client.Poll(ctx, "VM "+vm.Name+" deletion", func(ctx context.Context) (bool, error) {
		cur, err := r.client.GetVM(ctx, id)
		if client.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if cur.State == client.VMError {
			return false, fmt.Errorf("deleting VM %s failed: %s", cur.Name, cur.LastError)
		}
		return false, nil
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Waiting for VM deletion", err)
	}
}

func (r *vmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
