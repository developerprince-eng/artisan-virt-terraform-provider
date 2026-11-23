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

type clusterData struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	Location            types.String `tfsdk:"location"`
	Status              types.String `tfsdk:"status"`
	StatusMessage       types.String `tfsdk:"status_message"`
	StorageStrategy     types.String `tfsdk:"storage_strategy"`
	QuorumMode          types.String `tfsdk:"quorum_mode"`
	HasQuorum           types.Bool   `tfsdk:"has_quorum"`
	NodeIDs             types.List   `tfsdk:"node_ids"`
	VMCount             types.Int64  `tfsdk:"vm_count"`
	CPUCores            types.Int64  `tfsdk:"cpu_cores"`
	MemoryMiB           types.Int64  `tfsdk:"memory_mib"`
	AllocatedVCPUs      types.Int64  `tfsdk:"allocated_vcpus"`
	AllocatedMemoryMiB  types.Int64  `tfsdk:"allocated_memory_mib"`
	StorageGiB          types.Int64  `tfsdk:"storage_gib"`
	StorageAvailableGiB types.Int64  `tfsdk:"storage_available_gib"`
}

var clusterAttrTypes = map[string]attr.Type{
	"id": types.StringType, "name": types.StringType, "description": types.StringType, "location": types.StringType,
	"status": types.StringType, "status_message": types.StringType, "storage_strategy": types.StringType,
	"quorum_mode": types.StringType, "has_quorum": types.BoolType, "node_ids": types.ListType{ElemType: types.StringType},
	"vm_count": types.Int64Type, "cpu_cores": types.Int64Type, "memory_mib": types.Int64Type,
	"allocated_vcpus": types.Int64Type, "allocated_memory_mib": types.Int64Type,
	"storage_gib": types.Int64Type, "storage_available_gib": types.Int64Type,
}

func toClusterData(ctx context.Context, c *client.Cluster, diags *diag.Diagnostics) clusterData {
	ids := make([]string, 0, len(c.Nodes))
	for _, n := range c.Nodes {
		ids = append(ids, n.ID)
	}
	return clusterData{
		ID: types.StringValue(c.ID), Name: types.StringValue(c.Name), Description: types.StringValue(c.Description),
		Location: types.StringValue(c.Location), Status: types.StringValue(c.Status),
		StatusMessage: types.StringValue(c.StatusMessage), StorageStrategy: types.StringValue(c.StorageStrategy),
		QuorumMode: types.StringValue(c.Quorum.Mode), HasQuorum: types.BoolValue(c.QuorumState.HasQuorum),
		NodeIDs: stringList(ctx, ids, diags), VMCount: types.Int64Value(int64(c.VMCount)),
		CPUCores: types.Int64Value(int64(c.Capacity.CPUCores)), MemoryMiB: types.Int64Value(int64(c.Capacity.MemoryMiB)),
		AllocatedVCPUs:      types.Int64Value(int64(c.Capacity.AllocatedVCPUs)),
		AllocatedMemoryMiB:  types.Int64Value(int64(c.Capacity.AllocatedMemMiB)),
		StorageGiB:          types.Int64Value(int64(c.Capacity.StorageGiB)),
		StorageAvailableGiB: types.Int64Value(int64(c.Capacity.StorageAvailGiB)),
	}
}

func clusterSchemaAttrs(lookup bool) map[string]schema.Attribute {
	str := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	i64 := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Computed: true, MarkdownDescription: desc}
	}
	attrs := map[string]schema.Attribute{
		"id": str("Cluster id."), "name": str("Cluster name."), "description": str(""), "location": str(""),
		"status":           str("`creating`, `operational`, `degraded`, `error` or `deleting`."),
		"status_message":   str(""),
		"storage_strategy": str("`local`, `replicated` or `shared`."),
		"quorum_mode":      str("`majority` or `qdevice`."),
		"has_quorum":       schema.BoolAttribute{Computed: true},
		"node_ids":         schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Member node ids."},
		"vm_count":         i64(""), "cpu_cores": i64("Total CPU cores."), "memory_mib": i64("Total memory."),
		"allocated_vcpus": i64(""), "allocated_memory_mib": i64(""),
		"storage_gib": i64(""), "storage_available_gib": i64(""),
	}
	if lookup {
		attrs["id"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Cluster id. Exactly one of `id` or `name` is required."}
		attrs["name"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Cluster name."}
	}
	return attrs
}

// --- artisanvirt_cluster -----------------------------------------------------

type clusterDataSource struct{ dsBase }

func newClusterDataSource() datasource.DataSource { return &clusterDataSource{} }

func (d *clusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (d *clusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a cluster, with members, quorum and capacity, by id or name.",
		Attributes:          clusterSchemaAttrs(true),
	}
}

func (d *clusterDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *clusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterData
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Cluster
	if id := cfg.ID.ValueString(); id != "" {
		c, err := d.client.GetCluster(ctx, id)
		if err != nil {
			apiErr(&resp.Diagnostics, "Reading cluster "+id, err)
			return
		}
		found = c
	} else {
		cs, err := d.client.ListClusters(ctx)
		if err != nil {
			apiErr(&resp.Diagnostics, "Listing clusters", err)
			return
		}
		for _, c := range cs {
			if c.Name == cfg.Name.ValueString() {
				found = c
			}
		}
		if found == nil {
			resp.Diagnostics.AddError("Cluster not found", fmt.Sprintf("no cluster named %q", cfg.Name.ValueString()))
			return
		}
	}
	out := toClusterData(ctx, found, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}

// --- artisanvirt_clusters ----------------------------------------------------

type clustersDataSource struct{ dsBase }

func newClustersDataSource() datasource.DataSource { return &clustersDataSource{} }

type clustersModel struct {
	Clusters types.List `tfsdk:"clusters"`
}

func (d *clustersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clusters"
}

func (d *clustersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all clusters.",
		Attributes: map[string]schema.Attribute{
			"clusters": schema.ListNestedAttribute{
				Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: clusterSchemaAttrs(false)},
			},
		},
	}
}

func (d *clustersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	cs, err := d.client.ListClusters(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing clusters", err)
		return
	}
	out := make([]clusterData, 0, len(cs))
	for _, c := range cs {
		out = append(out, toClusterData(ctx, c, &resp.Diagnostics))
	}
	m := clustersModel{Clusters: objectList(ctx, clusterAttrTypes, out, &resp.Diagnostics)}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
