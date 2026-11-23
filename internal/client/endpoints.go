package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// --- tasks -----------------------------------------------------------------

func (c *Client) GetTask(ctx context.Context, id string) (*Task, error) {
	var t Task
	return &t, c.do(ctx, http.MethodGet, "/api/v1/tasks/"+esc(id), nil, nil, &t)
}

// --- enrollment tokens -----------------------------------------------------

func (c *Client) ListEnrollmentTokens(ctx context.Context) ([]*EnrollmentToken, error) {
	var out []*EnrollmentToken
	return out, c.do(ctx, http.MethodGet, "/api/v1/enrollment-tokens", nil, nil, &out)
}

func (c *Client) CreateEnrollmentToken(ctx context.Context, req CreateTokenRequest) (*CreatedToken, error) {
	var out CreatedToken
	return &out, c.do(ctx, http.MethodPost, "/api/v1/enrollment-tokens", nil, req, &out)
}

func (c *Client) RevokeEnrollmentToken(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/enrollment-tokens/"+esc(id), nil, nil, nil)
}

// --- nodes -----------------------------------------------------------------

func (c *Client) ListNodes(ctx context.Context) ([]*Node, error) {
	var out []*Node
	return out, c.do(ctx, http.MethodGet, "/api/v1/nodes", nil, nil, &out)
}

func (c *Client) GetNode(ctx context.Context, id string) (*Node, error) {
	var out Node
	return &out, c.do(ctx, http.MethodGet, "/api/v1/nodes/"+esc(id), nil, nil, &out)
}

func (c *Client) ApproveNode(ctx context.Context, id string) (*Node, error) {
	var out Node
	return &out, c.do(ctx, http.MethodPost, "/api/v1/nodes/"+esc(id)+"/approve", nil, nil, &out)
}

func (c *Client) SetMaintenance(ctx context.Context, id string, enabled bool) (*Node, error) {
	var out Node
	return &out, c.do(ctx, http.MethodPost, "/api/v1/nodes/"+esc(id)+"/maintenance", nil, map[string]bool{"enabled": enabled}, &out)
}

func (c *Client) DecommissionNode(ctx context.Context, id string, req DecommissionRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/nodes/"+esc(id)+"/decommission", nil, req, nil)
}

func (c *Client) RemoveNode(ctx context.Context, id, confirm string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/nodes/"+esc(id), url.Values{"confirm": {confirm}}, nil, nil)
}

func (c *Client) CreateBridge(ctx context.Context, nodeID string, req BridgeNetwork) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodPost, "/api/v1/nodes/"+esc(nodeID)+"/bridges", nil, req, &out)
}

func (c *Client) DeleteBridge(ctx context.Context, nodeID, name string) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodDelete, "/api/v1/nodes/"+esc(nodeID)+"/bridges/"+esc(name), url.Values{"confirm": {name}}, nil, &out)
}

// --- clusters --------------------------------------------------------------

func (c *Client) ListClusters(ctx context.Context) ([]*Cluster, error) {
	var out []*Cluster
	return out, c.do(ctx, http.MethodGet, "/api/v1/clusters", nil, nil, &out)
}

func (c *Client) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	var out Cluster
	return &out, c.do(ctx, http.MethodGet, "/api/v1/clusters/"+esc(id), nil, nil, &out)
}

func (c *Client) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	var out Cluster
	return &out, c.do(ctx, http.MethodPost, "/api/v1/clusters", nil, req, &out)
}

func (c *Client) DeleteCluster(ctx context.Context, id, confirm string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/clusters/"+esc(id), url.Values{"confirm": {confirm}}, nil, nil)
}

func (c *Client) JoinCluster(ctx context.Context, clusterID, nodeID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/clusters/"+esc(clusterID)+"/nodes", nil, map[string]string{"nodeId": nodeID}, nil)
}

func (c *Client) LeaveCluster(ctx context.Context, clusterID, nodeID, confirm string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/clusters/"+esc(clusterID)+"/nodes/"+esc(nodeID), url.Values{"confirm": {confirm}}, nil, nil)
}

// --- virtual machines ------------------------------------------------------

func (c *Client) ListVMs(ctx context.Context, clusterID, nodeID string) ([]*VM, error) {
	q := url.Values{}
	if clusterID != "" {
		q.Set("clusterId", clusterID)
	}
	if nodeID != "" {
		q.Set("nodeId", nodeID)
	}
	var out []*VM
	return out, c.do(ctx, http.MethodGet, "/api/v1/vms", q, nil, &out)
}

func (c *Client) GetVM(ctx context.Context, id string) (*VM, error) {
	var out VM
	return &out, c.do(ctx, http.MethodGet, "/api/v1/vms/"+esc(id), nil, nil, &out)
}

func (c *Client) CreateVM(ctx context.Context, req CreateVMRequest) (*VM, error) {
	var out VM
	return &out, c.do(ctx, http.MethodPost, "/api/v1/vms", nil, req, &out)
}

// DeleteVM returns the deletion task, or nil when the VM record was removed
// immediately (creation had failed on the host).
func (c *Client) DeleteVM(ctx context.Context, id, confirm string, deleteDisks bool) (*Task, error) {
	q := url.Values{"confirm": {confirm}, "deleteDisks": {strconv.FormatBool(deleteDisks)}}
	var out *Task
	if err := c.do(ctx, http.MethodDelete, "/api/v1/vms/"+esc(id), q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) VMAction(ctx context.Context, id, action string) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodPost, "/api/v1/vms/"+esc(id)+"/actions/"+esc(action), nil, nil, &out)
}

func (c *Client) CreateSnapshot(ctx context.Context, vmID string, req SnapshotRequest) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodPost, "/api/v1/vms/"+esc(vmID)+"/snapshots", nil, req, &out)
}

func (c *Client) DeleteSnapshot(ctx context.Context, vmID, name string) (*Task, error) {
	var out Task
	return &out, c.do(ctx, http.MethodDelete, "/api/v1/vms/"+esc(vmID)+"/snapshots/"+esc(name), nil, nil, &out)
}

func (c *Client) SetVMGPU(ctx context.Context, vmID, address string, attach bool) (*Task, error) {
	var out Task
	body := map[string]any{"address": address, "attach": attach}
	return &out, c.do(ctx, http.MethodPost, "/api/v1/vms/"+esc(vmID)+"/gpu", nil, body, &out)
}

// --- backups ---------------------------------------------------------------

func (c *Client) ListBackupTargets(ctx context.Context) ([]*BackupTarget, error) {
	var out []*BackupTarget
	return out, c.do(ctx, http.MethodGet, "/api/v1/backup-targets", nil, nil, &out)
}

// GetBackupTarget finds a target by id. The API has no GET-by-id route.
func (c *Client) GetBackupTarget(ctx context.Context, id string) (*BackupTarget, error) {
	ts, err := c.ListBackupTargets(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Code: "not_found", Message: "backup target " + id + " not found"}
}

func (c *Client) CreateBackupTarget(ctx context.Context, req CreateTargetRequest) (*BackupTarget, error) {
	var out BackupTarget
	return &out, c.do(ctx, http.MethodPost, "/api/v1/backup-targets", nil, req, &out)
}

func (c *Client) DeleteBackupTarget(ctx context.Context, id string, forget bool) error {
	var q url.Values
	if forget {
		q = url.Values{"forget": {"true"}}
	}
	return c.do(ctx, http.MethodDelete, "/api/v1/backup-targets/"+esc(id), q, nil, nil)
}

func (c *Client) ListBackupPolicies(ctx context.Context) ([]*BackupPolicy, error) {
	var out []*BackupPolicy
	return out, c.do(ctx, http.MethodGet, "/api/v1/backup-policies", nil, nil, &out)
}

// GetBackupPolicy finds a policy by id. The API has no GET-by-id route.
func (c *Client) GetBackupPolicy(ctx context.Context, id string) (*BackupPolicy, error) {
	ps, err := c.ListBackupPolicies(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Code: "not_found", Message: "backup policy " + id + " not found"}
}

func (c *Client) CreateBackupPolicy(ctx context.Context, req PolicyRequest) (*BackupPolicy, error) {
	var out BackupPolicy
	return &out, c.do(ctx, http.MethodPost, "/api/v1/backup-policies", nil, req, &out)
}

func (c *Client) UpdateBackupPolicy(ctx context.Context, id string, req PolicyRequest) (*BackupPolicy, error) {
	var out BackupPolicy
	return &out, c.do(ctx, http.MethodPut, "/api/v1/backup-policies/"+esc(id), nil, req, &out)
}

func (c *Client) DeleteBackupPolicy(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/backup-policies/"+esc(id), nil, nil, nil)
}

func (c *Client) ListBackups(ctx context.Context, vmID, targetID string) ([]*Backup, error) {
	q := url.Values{}
	if vmID != "" {
		q.Set("vmId", vmID)
	}
	if targetID != "" {
		q.Set("targetId", targetID)
	}
	var out []*Backup
	return out, c.do(ctx, http.MethodGet, "/api/v1/backups", q, nil, &out)
}
