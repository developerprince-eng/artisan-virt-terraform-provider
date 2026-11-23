package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*clusterMemberResource)(nil)
	_ resource.ResourceWithImportState = (*clusterMemberResource)(nil)
)

type clusterMemberResource struct{ base }

func newClusterMemberResource() resource.Resource { return &clusterMemberResource{} }

type clusterMemberModel struct {
	ID        types.String   `tfsdk:"id"`
	ClusterID types.String   `tfsdk:"cluster_id"`
	NodeID    types.String   `tfsdk:"node_id"`
	NodeName  types.String   `tfsdk:"node_name"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func (r *clusterMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_member"
}

func (r *clusterMemberResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Joins a ready node to an existing cluster after the join preflight passes. " +
			"Do not use this for the cluster's `initial_node_id`, which is a member already. Destroying the resource " +
			"removes the member; it must not host VMs and must not be the last member.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "`<cluster_id>/<node_id>`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"node_id":    schema.StringAttribute{Required: true, PlanModifiers: replace},
			"node_name":  schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

// waitMembership waits until the node's membership change finishes and
// reports whether it ended inside the cluster.
func (r *clusterMemberResource) waitMembership(ctx context.Context, nodeID, clusterID string) (*client.Node, bool, error) {
	var node *client.Node
	err := r.client.Poll(ctx, "cluster membership of "+nodeID, func(ctx context.Context) (bool, error) {
		n, err := r.client.GetNode(ctx, nodeID)
		if client.IsNotFound(err) {
			node = nil
			return true, nil
		}
		if err != nil {
			return false, err
		}
		node = n
		return n.Membership == "", nil
	})
	return node, node != nil && node.ClusterID == clusterID, err
}

func (r *clusterMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 60*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	clusterID, nodeID := plan.ClusterID.ValueString(), plan.NodeID.ValueString()
	if err := r.client.JoinCluster(ctx, clusterID, nodeID); err != nil {
		apiErr(&resp.Diagnostics, "Joining node to cluster", err)
		return
	}
	node, member, err := r.waitMembership(ctx, nodeID, clusterID)
	if err != nil {
		apiErr(&resp.Diagnostics, "Waiting for cluster join", err)
		return
	}
	if !member {
		msg := "the node left the cluster again"
		if node != nil && node.StateMessage != "" {
			msg = node.StateMessage
		}
		resp.Diagnostics.AddError("Cluster join failed",
			fmt.Sprintf("node %s did not join cluster %s: %s. Check `GET /api/v1/tasks?resourceId=%s` for the job log.", nodeID, clusterID, msg, nodeID))
		return
	}
	plan.ID = types.StringValue(clusterID + "/" + nodeID)
	plan.NodeName = types.StringValue(node.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	n, err := r.client.GetNode(ctx, state.NodeID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading node", err)
		return
	}
	if n.ClusterID != state.ClusterID.ValueString() {
		resp.State.RemoveResource(ctx)
		return
	}
	state.NodeName = types.StringValue(n.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 60*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	clusterID, nodeID := state.ClusterID.ValueString(), state.NodeID.ValueString()
	n, err := r.client.GetNode(ctx, nodeID)
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading node", err)
		return
	}
	if n.ClusterID != clusterID {
		return // already gone, e.g. the cluster was destroyed first
	}
	if err := r.client.LeaveCluster(ctx, clusterID, nodeID, n.Name); err != nil && !client.IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Removing node "+n.Name+" from cluster", err)
		return
	}
	_, member, err := r.waitMembership(ctx, nodeID, clusterID)
	if err != nil {
		apiErr(&resp.Diagnostics, "Waiting for node to leave the cluster", err)
		return
	}
	if member {
		resp.Diagnostics.AddError("Leaving the cluster failed",
			fmt.Sprintf("node %s is still a member of %s; check the cluster.leave task log.", n.Name, clusterID))
	}
}

func (r *clusterMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID, nodeID, ok := strings.Cut(req.ID, "/")
	if !ok || clusterID == "" || nodeID == "" {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <cluster_id>/<node_id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), nodeID)...)
}
