package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*nodeResource)(nil)
	_ resource.ResourceWithImportState = (*nodeResource)(nil)
)

// nodeResource adopts a hypervisor that enrolled itself with an enrollment
// token: it approves (provisions) the host and manages maintenance mode.
type nodeResource struct{ base }

func newNodeResource() resource.Resource { return &nodeResource{} }

type nodeModel struct {
	ID                    types.String   `tfsdk:"id"`
	NodeID                types.String   `tfsdk:"node_id"`
	Maintenance           types.Bool     `tfsdk:"maintenance"`
	DecommissionOnDestroy types.Bool     `tfsdk:"decommission_on_destroy"`
	PurgeStackOnDestroy   types.Bool     `tfsdk:"purge_stack_on_destroy"`
	ForceDecommission     types.Bool     `tfsdk:"force_decommission"`
	Name                  types.String   `tfsdk:"name"`
	State                 types.String   `tfsdk:"state"`
	StateMessage          types.String   `tfsdk:"state_message"`
	Address               types.String   `tfsdk:"address"`
	ClusterID             types.String   `tfsdk:"cluster_id"`
	Online                types.Bool     `tfsdk:"online"`
	Timeouts              timeouts.Value `tfsdk:"timeouts"`
}

func (r *nodeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (r *nodeResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Approves an enrolled hypervisor and manages its maintenance mode. Hosts enroll themselves " +
			"with an `artisanvirt_enrollment_token`; this resource then runs the host preflight and provisions the " +
			"virtualization stack, waiting until the node is `ready` (or `degraded`).\n\n" +
			"By default destroying this resource only removes it from Terraform state. Set `decommission_on_destroy` " +
			"to stop and remove the agent and drop the node from inventory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"node_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Id of the enrolled node (see the `artisanvirt_nodes` data source).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"maintenance": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Put the node in maintenance mode. Defaults to `false`.",
			},
			"decommission_on_destroy": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "On destroy, decommission the host (remove the agent and drop it from inventory). " +
					"The node must not be clustered or host VMs. Defaults to `false`.",
			},
			"purge_stack_on_destroy": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "When decommissioning, also remove the KVM/libvirt/Pacemaker/DRBD packages.",
			},
			"force_decommission": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "When decommissioning an unreachable host, forget it without cleaning it.",
			},
			"name":          schema.StringAttribute{Computed: true, MarkdownDescription: "Node name."},
			"state":         schema.StringAttribute{Computed: true, MarkdownDescription: "Node state."},
			"state_message": schema.StringAttribute{Computed: true, MarkdownDescription: "Detail for the current state."},
			"address":       schema.StringAttribute{Computed: true, MarkdownDescription: "Address the agent connects from."},
			"cluster_id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster the node belongs to, if any."},
			"online":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the agent heartbeat is recent."},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Update: true, Delete: true}),
		},
	}
}

func setNodeState(m *nodeModel, n *client.Node) {
	m.ID = types.StringValue(n.ID)
	m.NodeID = types.StringValue(n.ID)
	m.Name = types.StringValue(n.Name)
	m.State = types.StringValue(n.State)
	m.StateMessage = types.StringValue(n.StateMessage)
	m.Address = types.StringValue(n.Address)
	m.ClusterID = types.StringValue(n.ClusterID)
	m.Online = types.BoolValue(n.Online)
}

// waitNodeSettled waits until provisioning finishes. A quarantined node failed
// its provisioning playbook.
func (r *nodeResource) waitNodeSettled(ctx context.Context, id string) (*client.Node, error) {
	var node *client.Node
	err := r.client.Poll(ctx, "node "+id+" provisioning", func(ctx context.Context) (bool, error) {
		n, err := r.client.GetNode(ctx, id)
		if err != nil {
			return false, err
		}
		node = n
		switch n.State {
		case client.NodeReady, client.NodeDegraded, client.NodeMaintenance:
			return true, nil
		case client.NodeQuarantined, client.NodeRejected:
			return false, fmt.Errorf("node %s is %s: %s", n.Name, n.State, n.StateMessage)
		}
		return false, nil
	})
	return node, err
}

func (r *nodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 60*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id := plan.NodeID.ValueString()
	n, err := r.client.GetNode(ctx, id)
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading node "+id, err)
		return
	}
	if n.State == client.NodeDiscovered {
		if _, err := r.client.ApproveNode(ctx, id); err != nil {
			apiErr(&resp.Diagnostics, "Approving node "+n.Name, err)
			return
		}
	}
	if n, err = r.waitNodeSettled(ctx, id); err != nil {
		apiErr(&resp.Diagnostics, "Waiting for node provisioning", err)
		return
	}
	if want := plan.Maintenance.ValueBool(); want != (n.State == client.NodeMaintenance) {
		if n, err = r.client.SetMaintenance(ctx, id, want); err != nil {
			apiErr(&resp.Diagnostics, "Setting maintenance mode", err)
			return
		}
		if n, err = r.client.GetNode(ctx, id); err != nil {
			apiErr(&resp.Diagnostics, "Reading node "+id, err)
			return
		}
	}
	setNodeState(&plan, n)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	n, err := r.client.GetNode(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading node", err)
		return
	}
	setNodeState(&state, n)
	state.Maintenance = types.BoolValue(n.State == client.NodeMaintenance)
	// After import the destroy-time flags are null; store their defaults.
	for _, b := range []*types.Bool{&state.DecommissionOnDestroy, &state.PurgeStackOnDestroy, &state.ForceDecommission} {
		if b.IsNull() {
			*b = types.BoolValue(false)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nodeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Update(ctx, 20*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id := state.ID.ValueString()
	if plan.Maintenance.ValueBool() != state.Maintenance.ValueBool() {
		if _, err := r.client.SetMaintenance(ctx, id, plan.Maintenance.ValueBool()); err != nil {
			apiErr(&resp.Diagnostics, "Setting maintenance mode", err)
			return
		}
	}
	n, err := r.client.GetNode(ctx, id)
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading node", err)
		return
	}
	setNodeState(&plan, n)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nodeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !state.DecommissionOnDestroy.ValueBool() {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 60*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id := state.ID.ValueString()
	n, err := r.client.GetNode(ctx, id)
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading node", err)
		return
	}
	err = r.client.DecommissionNode(ctx, id, client.DecommissionRequest{
		Confirm:    n.Name,
		PurgeStack: state.PurgeStackOnDestroy.ValueBool(),
		Force:      state.ForceDecommission.ValueBool(),
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Decommissioning node "+n.Name, err)
		return
	}
	err = r.client.Poll(ctx, "node "+n.Name+" decommission", func(ctx context.Context) (bool, error) {
		cur, err := r.client.GetNode(ctx, id)
		if client.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if cur.State != client.NodeDecommissioning {
			return false, fmt.Errorf("decommission of %s stopped in state %s: %s", cur.Name, cur.State, cur.StateMessage)
		}
		return false, nil
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Waiting for node decommission", err)
	}
}

func (r *nodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), req.ID)...)
}
