package client

import (
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// Tasks and events
// ---------------------------------------------------------------------------

const (
	TaskQueued     = "queued"
	TaskDispatched = "dispatched"
	TaskRunning    = "running"
	TaskSucceeded  = "succeeded"
	TaskFailed     = "failed"
)

type Task struct {
	ID           string          `json:"id"`
	Executor     string          `json:"executor"`
	NodeID       string          `json:"nodeId,omitempty"`
	Type         string          `json:"type"`
	Params       json.RawMessage `json:"params,omitempty"`
	Status       string          `json:"status"`
	Progress     int             `json:"progress"`
	Message      string          `json:"message,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
	Error        string          `json:"error,omitempty"`
	Log          []string        `json:"log,omitempty"`
	ResourceKind string          `json:"resourceKind"`
	ResourceID   string          `json:"resourceId"`
	CreatedAt    time.Time       `json:"createdAt"`
	FinishedAt   *time.Time      `json:"finishedAt,omitempty"`
}

func (t *Task) Done() bool { return t.Status == TaskSucceeded || t.Status == TaskFailed }

// ---------------------------------------------------------------------------
// Enrollment tokens
// ---------------------------------------------------------------------------

type EnrollmentToken struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	UsedAt      *time.Time `json:"usedAt,omitempty"`
	UsedByNode  string     `json:"usedByNode,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	CreatedBy   string     `json:"createdBy"`
}

type CreateTokenRequest struct {
	Description string `json:"description,omitempty"`
	TTLMinutes  int    `json:"ttlMinutes,omitempty"`
}

type CreatedToken struct {
	Token *EnrollmentToken `json:"token"`
	Value string           `json:"value"`
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

const (
	NodeDiscovered      = "discovered"
	NodeProvisioning    = "provisioning"
	NodeReady           = "ready"
	NodeMaintenance     = "maintenance"
	NodeDegraded        = "degraded"
	NodeOffline         = "offline"
	NodeQuarantined     = "quarantined"
	NodeRejected        = "rejected"
	NodeDecommissioning = "decommissioning"
)

type StoragePool struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Path      string `json:"path"`
	Capacity  uint64 `json:"capacityBytes"`
	Available uint64 `json:"availableBytes"`
	Active    bool   `json:"active"`
}

type BridgeNetwork struct {
	Name      string `json:"name"`
	Mode      string `json:"mode,omitempty"`
	IPv4      string `json:"ipv4,omitempty"`
	DHCPStart string `json:"dhcpStart,omitempty"`
	DHCPEnd   string `json:"dhcpEnd,omitempty"`
}

type NodeInventory struct {
	Hostname       string          `json:"hostname"`
	OS             string          `json:"os"`
	OSVersion      string          `json:"osVersion"`
	Kernel         string          `json:"kernel"`
	Arch           string          `json:"arch"`
	CPUModel       string          `json:"cpuModel"`
	CPUVendor      string          `json:"cpuVendor"`
	CPUCores       int             `json:"cpuCores"`
	KVMAvailable   bool            `json:"kvmAvailable"`
	MemoryBytes    uint64          `json:"memoryBytes"`
	MemoryFree     uint64          `json:"memoryFreeBytes"`
	StoragePools   []StoragePool   `json:"storagePools"`
	Bridges        []string        `json:"bridges"`
	BridgeNetworks []BridgeNetwork `json:"bridgeNetworks,omitempty"`
	Networks       []string        `json:"networks"`
	Addresses      []string        `json:"addresses"`
	LibvirtVersion string          `json:"libvirtVersion"`
	DRBDAvailable  bool            `json:"drbdAvailable"`
	AgentVersion   string          `json:"agentVersion"`
	Driver         string          `json:"driver"`
}

type Allocation struct {
	VMs       int `json:"vms"`
	VCPUs     int `json:"vcpus"`
	MemoryMiB int `json:"memoryMiB"`
	DiskGiB   int `json:"diskGiB"`
}

// Node is the NodeView returned by GET /nodes and GET /nodes/{id}.
type Node struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	State         string            `json:"state"`
	StateMessage  string            `json:"stateMessage,omitempty"`
	ClusterID     string            `json:"clusterId,omitempty"`
	ClusterName   string            `json:"clusterName,omitempty"`
	Membership    string            `json:"membership,omitempty"`
	Address       string            `json:"address"`
	Labels        map[string]string `json:"labels,omitempty"`
	Inventory     NodeInventory     `json:"inventory"`
	Allocation    Allocation        `json:"allocation"`
	Online        bool              `json:"online"`
	LastHeartbeat time.Time         `json:"lastHeartbeat"`
	EnrolledAt    time.Time         `json:"enrolledAt"`
	ApprovedAt    *time.Time        `json:"approvedAt,omitempty"`
}

type DecommissionRequest struct {
	Confirm    string `json:"confirm"`
	PurgeStack bool   `json:"purgeStack"`
	Force      bool   `json:"force"`
}

// ---------------------------------------------------------------------------
// Clusters
// ---------------------------------------------------------------------------

const (
	ClusterCreating    = "creating"
	ClusterOperational = "operational"
	ClusterDegraded    = "degraded"
	ClusterError       = "error"
	ClusterDeleting    = "deleting"
)

type ClusterNetwork struct {
	RingSubnet string `json:"ringSubnet"`
	Transport  string `json:"transport"`
}

type QuorumConfig struct {
	Mode        string `json:"mode"`
	QDeviceHost string `json:"qdeviceHost,omitempty"`
}

type FencingConfig struct {
	Enabled bool   `json:"enabled"`
	Agent   string `json:"agent,omitempty"`
}

type QuorumState struct {
	Members   int  `json:"members"`
	Online    int  `json:"online"`
	Required  int  `json:"required"`
	HasQuorum bool `json:"hasQuorum"`
	Expected  int  `json:"expectedVotes"`
}

type Capacity struct {
	CPUCores        int `json:"cpuCores"`
	MemoryMiB       int `json:"memoryMiB"`
	AllocatedVCPUs  int `json:"allocatedVcpus"`
	AllocatedMemMiB int `json:"allocatedMemoryMiB"`
	StorageGiB      int `json:"storageGiB"`
	StorageAvailGiB int `json:"storageAvailableGiB"`
}

// Cluster is the ClusterView returned by GET; POST returns the same object
// without nodes, vmCount, quorumState and capacity.
type Cluster struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	Location        string         `json:"location"`
	Status          string         `json:"status"`
	StatusMessage   string         `json:"statusMessage,omitempty"`
	Network         ClusterNetwork `json:"network"`
	Quorum          QuorumConfig   `json:"quorum"`
	Fencing         FencingConfig  `json:"fencing"`
	StorageStrategy string         `json:"storageStrategy"`
	Nodes           []*Node        `json:"nodes,omitempty"`
	VMCount         int            `json:"vmCount"`
	QuorumState     QuorumState    `json:"quorumState"`
	Capacity        Capacity       `json:"capacity"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type CreateClusterRequest struct {
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	Location        string         `json:"location"`
	InitialNodeID   string         `json:"initialNodeId"`
	Network         ClusterNetwork `json:"network"`
	Quorum          QuorumConfig   `json:"quorum"`
	Fencing         FencingConfig  `json:"fencing"`
	StorageStrategy string         `json:"storageStrategy"`
}

type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Warning bool   `json:"warning,omitempty"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Virtual machines
// ---------------------------------------------------------------------------

const (
	VMPending   = "pending"
	VMCreating  = "creating"
	VMRunning   = "running"
	VMStopped   = "stopped"
	VMPaused    = "paused"
	VMRestoring = "restoring"
	VMDeleting  = "deleting"
	VMError     = "error"
	VMUnknown   = "unknown"
)

type DiskSpec struct {
	Device      string `json:"device"`
	SizeGiB     int    `json:"sizeGiB"`
	Pool        string `json:"pool"`
	SourceImage string `json:"sourceImage,omitempty"`
	Path        string `json:"path,omitempty"`
}

type NICSpec struct {
	Network string `json:"network,omitempty"`
	Bridge  string `json:"bridge,omitempty"`
	Model   string `json:"model,omitempty"`
	MAC     string `json:"mac,omitempty"`
}

type GPUAssignment struct {
	Address          string   `json:"address"`
	Devices          []string `json:"devices"`
	PhysicalFunction string   `json:"physicalFunction,omitempty"`
}

type VMSpec struct {
	Name      string         `json:"name"`
	UUID      string         `json:"uuid"`
	VCPUs     int            `json:"vcpus"`
	MemoryMiB int            `json:"memoryMiB"`
	Firmware  string         `json:"firmware"`
	Machine   string         `json:"machine,omitempty"`
	CPUMode   string         `json:"cpuMode,omitempty"`
	BootISO   string         `json:"bootIso,omitempty"`
	Disks     []DiskSpec     `json:"disks"`
	NICs      []NICSpec      `json:"nics"`
	Autostart bool           `json:"autostart"`
	GPU       *GPUAssignment `json:"gpu,omitempty"`
}

type Snapshot struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type VM struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	ClusterID    string            `json:"clusterId,omitempty"`
	NodeID       string            `json:"nodeId"`
	State        string            `json:"state"`
	LastError    string            `json:"lastError,omitempty"`
	Spec         VMSpec            `json:"spec"`
	Tags         map[string]string `json:"tags,omitempty"`
	Snapshots    []Snapshot        `json:"snapshots"`
	Defined      bool              `json:"defined"`
	RestoredFrom string            `json:"restoredFrom,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type CreateDiskRequest struct {
	SizeGiB     int    `json:"sizeGiB"`
	Pool        string `json:"pool,omitempty"`
	SourceImage string `json:"sourceImage,omitempty"`
}

type CreateVMRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	ClusterID   string              `json:"clusterId,omitempty"`
	NodeID      string              `json:"nodeId,omitempty"`
	VCPUs       int                 `json:"vcpus"`
	MemoryMiB   int                 `json:"memoryMiB"`
	Firmware    string              `json:"firmware,omitempty"`
	CPUMode     string              `json:"cpuMode,omitempty"`
	BootISO     string              `json:"bootIso,omitempty"`
	Disks       []CreateDiskRequest `json:"disks"`
	NICs        []NICSpec           `json:"nics,omitempty"`
	Tags        map[string]string   `json:"tags,omitempty"`
	Start       bool                `json:"start"`
	Autostart   bool                `json:"autostart"`
}

type SnapshotRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ---------------------------------------------------------------------------
// Backups
// ---------------------------------------------------------------------------

type BackupTarget struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Path        string    `json:"path"`
	NFSServer   string    `json:"nfsServer,omitempty"`
	NFSExport   string    `json:"nfsExport,omitempty"`
	ClusterIDs  []string  `json:"clusterIds"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CreateTargetRequest struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Path        string   `json:"path"`
	NFSServer   string   `json:"nfsServer,omitempty"`
	NFSExport   string   `json:"nfsExport,omitempty"`
	ClusterIDs  []string `json:"clusterIds"`
	Description string   `json:"description,omitempty"`
}

type BackupSelector struct {
	VMIDs      []string          `json:"vmIds,omitempty"`
	ClusterIDs []string          `json:"clusterIds,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type BackupPolicy struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Schedule  string         `json:"schedule"`
	TargetID  string         `json:"targetId"`
	KeepLast  int            `json:"keepLast"`
	Selector  BackupSelector `json:"selector"`
	LastRunAt *time.Time     `json:"lastRunAt,omitempty"`
	NextRunAt *time.Time     `json:"nextRunAt,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type PolicyRequest struct {
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Schedule string         `json:"schedule"`
	TargetID string         `json:"targetId"`
	KeepLast int            `json:"keepLast"`
	Selector BackupSelector `json:"selector"`
}

type Backup struct {
	ID          string     `json:"id"`
	VMID        string     `json:"vmId"`
	VMName      string     `json:"vmName"`
	ClusterID   string     `json:"clusterId,omitempty"`
	NodeID      string     `json:"nodeId"`
	TargetID    string     `json:"targetId"`
	PolicyID    string     `json:"policyId,omitempty"`
	Status      string     `json:"status"`
	SizeBytes   uint64     `json:"sizeBytes"`
	Consistency string     `json:"consistency,omitempty"`
	TaskID      string     `json:"taskId"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
