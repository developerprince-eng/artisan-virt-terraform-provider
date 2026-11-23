package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*clusterResource)(nil)
	_ resource.ResourceWithImportState = (*clusterResource)(nil)
)

type clusterResource struct{ base }

func newClusterResource() resource.Resource { return &clusterResource{} }

type clusterModel struct {
	ID              types.String   `tfsdk:"id"`
	Name            types.String   `tfsdk:"name"`
	Description     types.String   `tfsdk:"description"`
	Location        types.String   `tfsdk:"location"`
	InitialNodeID   types.String   `tfsdk:"initial_node_id"`
	RingSubnet      types.String   `tfsdk:"ring_subnet"`
	Transport       types.String   `tfsdk:"transport"`
	QuorumMode      types.String   `tfsdk:"quorum_mode"`
	QDeviceHost     types.String   `tfsdk:"qdevice_host"`
	FencingEnabled  types.Bool     `tfsdk:"fencing_enabled"`
	FencingAgent    types.String   `tfsdk:"fencing_agent"`
	StorageStrategy types.String   `tfsdk:"storage_strategy"`
	Status          types.String   `tfsdk:"status"`
	StatusMessage   types.String   `tfsdk:"status_message"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *clusterResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Corosync/Pacemaker cluster created on an initial hypervisor. Add more members with " +
			"`artisanvirt_cluster_member`. The API has no update operation, so any change replaces the cluster; " +
			"a cluster can only be destroyed when it contains no VMs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{
				Required: true, PlanModifiers: replace, MarkdownDescription: "Cluster name.",
				Validators: []validator.String{nameValidator()},
			},
			"description":     schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"location":        schema.StringAttribute{Optional: true, PlanModifiers: replace, MarkdownDescription: "Datacenter / region label, e.g. `fsn1`."},
			"initial_node_id": schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Ready node the cluster is bootstrapped on."},
			"ring_subnet": schema.StringAttribute{
				Optional: true, PlanModifiers: replace,
				MarkdownDescription: "CIDR for the Corosync ring, for example `10.10.0.0/24`.",
			},
			"transport": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("knet"), PlanModifiers: replace,
				MarkdownDescription: "Corosync transport. Defaults to `knet`.",
			},
			"quorum_mode": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("majority"), PlanModifiers: replace,
				MarkdownDescription: "`majority` (default) or `qdevice`.",
				Validators:          []validator.String{stringvalidator.OneOf("majority", "qdevice")},
			},
			"qdevice_host": schema.StringAttribute{
				Optional: true, PlanModifiers: replace, MarkdownDescription: "QDevice host; required when `quorum_mode = \"qdevice\"`.",
			},
			"fencing_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Enable STONITH fencing. Defaults to `false`.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"fencing_agent": schema.StringAttribute{
				Optional: true, PlanModifiers: replace, MarkdownDescription: "Fence agent, e.g. `fence_ipmilan`; required when fencing is enabled.",
			},
			"storage_strategy": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("local"), PlanModifiers: replace,
				MarkdownDescription: "`local` (default), `replicated` (DRBD) or `shared`.",
				Validators:          []validator.String{stringvalidator.OneOf("local", "replicated", "shared")},
			},
			"status":         schema.StringAttribute{Computed: true, PlanModifiers: keep, MarkdownDescription: "Cluster status."},
			"status_message": schema.StringAttribute{Computed: true, MarkdownDescription: "Status detail."},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

func setClusterState(m *clusterModel, c *client.Cluster) {
	m.ID = types.StringValue(c.ID)
	m.Name = types.StringValue(c.Name)
	m.Description = keepNullString(m.Description, c.Description)
	m.Location = keepNullString(m.Location, c.Location)
	m.RingSubnet = keepNullString(m.RingSubnet, c.Network.RingSubnet)
	m.Transport = types.StringValue(orDefault(c.Network.Transport, "knet"))
	m.QuorumMode = types.StringValue(orDefault(c.Quorum.Mode, "majority"))
	m.QDeviceHost = keepNullString(m.QDeviceHost, c.Quorum.QDeviceHost)
	m.FencingEnabled = types.BoolValue(c.Fencing.Enabled)
	m.FencingAgent = keepNullString(m.FencingAgent, c.Fencing.Agent)
	m.StorageStrategy = types.StringValue(orDefault(c.StorageStrategy, "local"))
	m.Status = types.StringValue(c.Status)
	m.StatusMessage = types.StringValue(c.StatusMessage)
}

// keepNullString keeps an optional attribute null when the API reports "".
func keepNullString(prior types.String, got string) types.String {
	if got == "" && (prior.IsNull() || prior.IsUnknown()) {
		return types.StringNull()
	}
	return types.StringValue(got)
}

func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 60*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c, err := r.client.CreateCluster(ctx, client.CreateClusterRequest{
		Name:            plan.Name.ValueString(),
		Description:     plan.Description.ValueString(),
		Location:        plan.Location.ValueString(),
		InitialNodeID:   plan.InitialNodeID.ValueString(),
		Network:         client.ClusterNetwork{RingSubnet: plan.RingSubnet.ValueString(), Transport: plan.Transport.ValueString()},
		Quorum:          client.QuorumConfig{Mode: plan.QuorumMode.ValueString(), QDeviceHost: plan.QDeviceHost.ValueString()},
		Fencing:         client.FencingConfig{Enabled: plan.FencingEnabled.ValueBool(), Agent: plan.FencingAgent.ValueString()},
		StorageStrategy: plan.StorageStrategy.ValueString(),
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating cluster", err)
		return
	}
	setClusterState(&plan, c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...) // track it even if the wait fails

	err = r.client.Poll(ctx, "cluster "+c.Name, func(ctx context.Context) (bool, error) {
		cur, err := r.client.GetCluster(ctx, c.ID)
		if err != nil {
			return false, err
		}
		c = cur
		switch cur.Status {
		case client.ClusterOperational, client.ClusterDegraded:
			return true, nil
		case client.ClusterError:
			return false, fmt.Errorf("cluster %s failed: %s", cur.Name, cur.StatusMessage)
		}
		return false, nil
	})
	setClusterState(&plan, c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if err != nil {
		apiErr(&resp.Diagnostics, "Waiting for cluster creation", err)
	}
}

func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.client.GetCluster(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading cluster", err)
		return
	}
	setClusterState(&state, c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 60*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id := state.ID.ValueString()
	c, err := r.client.GetCluster(ctx, id)
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading cluster", err)
		return
	}
	if c.Status != client.ClusterDeleting {
		if err := r.client.DeleteCluster(ctx, id, c.Name); err != nil && !client.IsNotFound(err) {
			apiErr(&resp.Diagnostics, "Deleting cluster "+c.Name, err)
			return
		}
	}
	err = r.client.Poll(ctx, "cluster "+c.Name+" deletion", func(ctx context.Context) (bool, error) {
		cur, err := r.client.GetCluster(ctx, id)
		if client.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if cur.Status == client.ClusterError {
			return false, fmt.Errorf("cluster %s: %s", cur.Name, cur.StatusMessage)
		}
		return false, nil
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Waiting for cluster deletion", err)
	}
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
