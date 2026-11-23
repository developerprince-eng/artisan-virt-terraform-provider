package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

// nodeData is the flattened node shape shared by artisanvirt_node and
// artisanvirt_nodes.
type nodeData struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	State              types.String `tfsdk:"state"`
	StateMessage       types.String `tfsdk:"state_message"`
	ClusterID          types.String `tfsdk:"cluster_id"`
	ClusterName        types.String `tfsdk:"cluster_name"`
	Membership         types.String `tfsdk:"membership"`
	Address            types.String `tfsdk:"address"`
	Online             types.Bool   `tfsdk:"online"`
	Labels             types.Map    `tfsdk:"labels"`
	Hostname           types.String `tfsdk:"hostname"`
	OS                 types.String `tfsdk:"os"`
	OSVersion          types.String `tfsdk:"os_version"`
	Kernel             types.String `tfsdk:"kernel"`
	Arch               types.String `tfsdk:"arch"`
	CPUModel           types.String `tfsdk:"cpu_model"`
	CPUCores           types.Int64  `tfsdk:"cpu_cores"`
	MemoryBytes        types.Int64  `tfsdk:"memory_bytes"`
	KVMAvailable       types.Bool   `tfsdk:"kvm_available"`
	LibvirtVersion     types.String `tfsdk:"libvirt_version"`
	AgentVersion       types.String `tfsdk:"agent_version"`
	Bridges            types.List   `tfsdk:"bridges"`
	Networks           types.List   `tfsdk:"networks"`
	Addresses          types.List   `tfsdk:"addresses"`
	StoragePools       types.List   `tfsdk:"storage_pools"`
	VMCount            types.Int64  `tfsdk:"vm_count"`
	AllocatedVCPUs     types.Int64  `tfsdk:"allocated_vcpus"`
	AllocatedMemoryMiB types.Int64  `tfsdk:"allocated_memory_mib"`
	AllocatedDiskGiB   types.Int64  `tfsdk:"allocated_disk_gib"`
}

type storagePoolData struct {
	Name           types.String `tfsdk:"name"`
	Type           types.String `tfsdk:"type"`
	Path           types.String `tfsdk:"path"`
	CapacityBytes  types.Int64  `tfsdk:"capacity_bytes"`
	AvailableBytes types.Int64  `tfsdk:"available_bytes"`
	Active         types.Bool   `tfsdk:"active"`
}

var storagePoolAttrTypes = map[string]attr.Type{
	"name": types.StringType, "type": types.StringType, "path": types.StringType,
	"capacity_bytes": types.Int64Type, "available_bytes": types.Int64Type, "active": types.BoolType,
}

var nodeAttrTypes = map[string]attr.Type{
	"id": types.StringType, "name": types.StringType, "state": types.StringType, "state_message": types.StringType,
	"cluster_id": types.StringType, "cluster_name": types.StringType, "membership": types.StringType,
	"address": types.StringType, "online": types.BoolType, "labels": types.MapType{ElemType: types.StringType},
	"hostname": types.StringType, "os": types.StringType, "os_version": types.StringType, "kernel": types.StringType,
	"arch": types.StringType, "cpu_model": types.StringType, "cpu_cores": types.Int64Type,
	"memory_bytes": types.Int64Type, "kvm_available": types.BoolType, "libvirt_version": types.StringType,
	"agent_version": types.StringType,
	"bridges":       types.ListType{ElemType: types.StringType}, "networks": types.ListType{ElemType: types.StringType},
	"addresses":     types.ListType{ElemType: types.StringType},
	"storage_pools": types.ListType{ElemType: types.ObjectType{AttrTypes: storagePoolAttrTypes}},
	"vm_count":      types.Int64Type, "allocated_vcpus": types.Int64Type, "allocated_memory_mib": types.Int64Type,
	"allocated_disk_gib": types.Int64Type,
}

func toNodeData(ctx context.Context, n *client.Node, diags *diag.Diagnostics) nodeData {
	pools := make([]storagePoolData, 0, len(n.Inventory.StoragePools))
	for _, p := range n.Inventory.StoragePools {
		pools = append(pools, storagePoolData{
			Name: types.StringValue(p.Name), Type: types.StringValue(p.Type), Path: types.StringValue(p.Path),
			CapacityBytes: types.Int64Value(int64(p.Capacity)), AvailableBytes: types.Int64Value(int64(p.Available)),
			Active: types.BoolValue(p.Active),
		})
	}
	inv := n.Inventory
	return nodeData{
		ID: types.StringValue(n.ID), Name: types.StringValue(n.Name), State: types.StringValue(n.State),
		StateMessage: types.StringValue(n.StateMessage), ClusterID: types.StringValue(n.ClusterID),
		ClusterName: types.StringValue(n.ClusterName), Membership: types.StringValue(n.Membership),
		Address: types.StringValue(n.Address), Online: types.BoolValue(n.Online), Labels: stringMap(ctx, n.Labels, diags),
		Hostname: types.StringValue(inv.Hostname), OS: types.StringValue(inv.OS), OSVersion: types.StringValue(inv.OSVersion),
		Kernel: types.StringValue(inv.Kernel), Arch: types.StringValue(inv.Arch), CPUModel: types.StringValue(inv.CPUModel),
		CPUCores: types.Int64Value(int64(inv.CPUCores)), MemoryBytes: types.Int64Value(int64(inv.MemoryBytes)),
		KVMAvailable: types.BoolValue(inv.KVMAvailable), LibvirtVersion: types.StringValue(inv.LibvirtVersion),
		AgentVersion: types.StringValue(inv.AgentVersion),
		Bridges:      stringList(ctx, inv.Bridges, diags), Networks: stringList(ctx, inv.Networks, diags),
		Addresses:    stringList(ctx, inv.Addresses, diags),
		StoragePools: objectList(ctx, storagePoolAttrTypes, pools, diags),
		VMCount:      types.Int64Value(int64(n.Allocation.VMs)), AllocatedVCPUs: types.Int64Value(int64(n.Allocation.VCPUs)),
		AllocatedMemoryMiB: types.Int64Value(int64(n.Allocation.MemoryMiB)),
		AllocatedDiskGiB:   types.Int64Value(int64(n.Allocation.DiskGiB)),
	}
}

// nodeSchemaAttrs returns the computed node attributes; lookup marks id and
// name as optional inputs for the single-node data source.
func nodeSchemaAttrs(lookup bool) map[string]schema.Attribute {
	str := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	i64 := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Computed: true, MarkdownDescription: desc}
	}
	strs := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: desc}
	}
	attrs := map[string]schema.Attribute{
		"id":              str("Node id."),
		"name":            str("Node name."),
		"state":           str("`discovered`, `provisioning`, `ready`, `maintenance`, `degraded`, `offline`, `quarantined`, `rejected` or `decommissioning`."),
		"state_message":   str("Detail for the state."),
		"cluster_id":      str("Cluster id, empty for standalone nodes."),
		"cluster_name":    str("Cluster name."),
		"membership":      str("`joining` or `leaving` while a membership change runs."),
		"address":         str("Agent address."),
		"online":          schema.BoolAttribute{Computed: true, MarkdownDescription: "Heartbeat is recent."},
		"labels":          schema.MapAttribute{Computed: true, ElementType: types.StringType},
		"hostname":        str("Host name."),
		"os":              str("Operating system."),
		"os_version":      str("OS version."),
		"kernel":          str("Kernel version."),
		"arch":            str("CPU architecture."),
		"cpu_model":       str("CPU model."),
		"cpu_cores":       i64("CPU cores."),
		"memory_bytes":    i64("Physical memory."),
		"kvm_available":   schema.BoolAttribute{Computed: true},
		"libvirt_version": str("libvirt version."),
		"agent_version":   str("Agent version."),
		"bridges":         strs("Host bridges."),
		"networks":        strs("libvirt networks."),
		"addresses":       strs("Host IP addresses."),
		"storage_pools": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name": str(""), "type": str(""), "path": str(""),
			"capacity_bytes": i64(""), "available_bytes": i64(""), "active": schema.BoolAttribute{Computed: true},
		}}},
		"vm_count":             i64("VMs on the node."),
		"allocated_vcpus":      i64("vCPUs allocated to VMs."),
		"allocated_memory_mib": i64("Memory allocated to VMs."),
		"allocated_disk_gib":   i64("Disk allocated to VMs."),
	}
	if lookup {
		attrs["id"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Node id. Exactly one of `id` or `name` is required."}
		attrs["name"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Node name."}
	}
	return attrs
}

// --- artisanvirt_node --------------------------------------------------------

type nodeDataSource struct{ dsBase }

func newNodeDataSource() datasource.DataSource { return &nodeDataSource{} }

func (d *nodeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (d *nodeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up one hypervisor node, with inventory and allocation, by id or name.",
		Attributes:          nodeSchemaAttrs(true),
	}
}

func (d *nodeDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *nodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg nodeData
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var node *client.Node
	if id := cfg.ID.ValueString(); id != "" {
		n, err := d.client.GetNode(ctx, id)
		if err != nil {
			apiErr(&resp.Diagnostics, "Reading node "+id, err)
			return
		}
		node = n
	} else {
		ns, err := d.client.ListNodes(ctx)
		if err != nil {
			apiErr(&resp.Diagnostics, "Listing nodes", err)
			return
		}
		for _, n := range ns {
			if n.Name == cfg.Name.ValueString() {
				node = n
				break
			}
		}
		if node == nil {
			resp.Diagnostics.AddError("Node not found", fmt.Sprintf("no node named %q", cfg.Name.ValueString()))
			return
		}
	}
	out := toNodeData(ctx, node, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}

// --- artisanvirt_nodes -------------------------------------------------------

type nodesDataSource struct{ dsBase }

func newNodesDataSource() datasource.DataSource { return &nodesDataSource{} }

type nodesModel struct {
	State     types.String `tfsdk:"state"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Nodes     types.List   `tfsdk:"nodes"`
}

func (d *nodesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nodes"
}

func (d *nodesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists hypervisor nodes, optionally filtered by state or cluster. Use `state = \"discovered\"` " +
			"to find hosts that enrolled and wait for approval.",
		Attributes: map[string]schema.Attribute{
			"state":      schema.StringAttribute{Optional: true, MarkdownDescription: "Only nodes in this state."},
			"cluster_id": schema.StringAttribute{Optional: true, MarkdownDescription: "Only members of this cluster."},
			"nodes": schema.ListNestedAttribute{
				Computed:     true,
				NestedObject: schema.NestedAttributeObject{Attributes: nodeSchemaAttrs(false)},
			},
		},
	}
}

func (d *nodesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg nodesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ns, err := d.client.ListNodes(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing nodes", err)
		return
	}
	out := []nodeData{}
	for _, n := range ns {
		if s := cfg.State.ValueString(); s != "" && n.State != s {
			continue
		}
		if c := cfg.ClusterID.ValueString(); c != "" && n.ClusterID != c {
			continue
		}
		out = append(out, toNodeData(ctx, n, &resp.Diagnostics))
	}
	cfg.Nodes = objectList(ctx, nodeAttrTypes, out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
