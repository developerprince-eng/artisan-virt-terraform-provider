package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*bridgeResource)(nil)
	_ resource.ResourceWithImportState = (*bridgeResource)(nil)
)

type bridgeResource struct{ base }

func newBridgeResource() resource.Resource { return &bridgeResource{} }

type bridgeModel struct {
	ID        types.String   `tfsdk:"id"`
	NodeID    types.String   `tfsdk:"node_id"`
	Name      types.String   `tfsdk:"name"`
	Mode      types.String   `tfsdk:"mode"`
	IPv4      types.String   `tfsdk:"ipv4"`
	DHCPStart types.String   `tfsdk:"dhcp_start"`
	DHCPEnd   types.String   `tfsdk:"dhcp_end"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func (r *bridgeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bridge"
}

func (r *bridgeResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A persistent host-local libvirt bridge on a node, in `isolated` or `nat` mode, " +
			"optionally with an IPv4 address and DHCP range. NAT and DHCP require `ipv4`. " +
			"Bridges cannot be changed in place; any change replaces the bridge, which fails while VMs use it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "`<node_id>/<name>`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"node_id": schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Node to create the bridge on."},
			"name": schema.StringAttribute{
				Required: true, PlanModifiers: replace, MarkdownDescription: "Bridge interface name (max 15 characters).",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 15)},
			},
			"mode": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("isolated"), PlanModifiers: replace,
				MarkdownDescription: "`isolated` (default) or `nat`.",
				Validators:          []validator.String{stringvalidator.OneOf("isolated", "nat")},
			},
			"ipv4": schema.StringAttribute{
				Optional: true, PlanModifiers: replace,
				MarkdownDescription: "Bridge address with prefix, for example `192.168.100.1/24`.",
			},
			"dhcp_start": schema.StringAttribute{Optional: true, PlanModifiers: replace, MarkdownDescription: "First DHCP lease address."},
			"dhcp_end":   schema.StringAttribute{Optional: true, PlanModifiers: replace, MarkdownDescription: "Last DHCP lease address."},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

func (r *bridgeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bridgeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	nodeID, name := plan.NodeID.ValueString(), plan.Name.ValueString()
	task, err := r.client.CreateBridge(ctx, nodeID, client.BridgeNetwork{
		Name:      name,
		Mode:      plan.Mode.ValueString(),
		IPv4:      plan.IPv4.ValueString(),
		DHCPStart: plan.DHCPStart.ValueString(),
		DHCPEnd:   plan.DHCPEnd.ValueString(),
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating bridge "+name, err)
		return
	}
	if _, err := r.client.WaitTask(ctx, task.ID); err != nil {
		apiErr(&resp.Diagnostics, "Creating bridge "+name, err)
		return
	}
	plan.ID = types.StringValue(nodeID + "/" + name)
	// Save now so a later wait failure does not orphan the bridge.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	// The bridge shows up in host inventory with the next agent heartbeat.
	err = r.client.Poll(ctx, "bridge "+name+" in host inventory", func(ctx context.Context) (bool, error) {
		n, err := r.client.GetNode(ctx, nodeID)
		if err != nil {
			return false, err
		}
		return slices.Contains(n.Inventory.Bridges, name), nil
	})
	if err != nil {
		resp.Diagnostics.AddWarning("Bridge not yet reported by the host", err.Error())
	}
}

func (r *bridgeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bridgeModel
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
	name := state.Name.ValueString()
	for _, b := range n.Inventory.BridgeNetworks {
		if b.Name == name {
			state.Mode = types.StringValue(orDefault(b.Mode, "isolated"))
			state.IPv4 = optString(b.IPv4)
			state.DHCPStart = optString(b.DHCPStart)
			state.DHCPEnd = optString(b.DHCPEnd)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	if !slices.Contains(n.Inventory.Bridges, name) {
		resp.State.RemoveResource(ctx)
		return
	}
	// Present but without Artisan Virt metadata (older agent): keep state as is.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *bridgeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan bridgeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bridgeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bridgeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	task, err := r.client.DeleteBridge(ctx, state.NodeID.ValueString(), state.Name.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Deleting bridge", err)
		return
	}
	if _, err := r.client.WaitTask(ctx, task.ID); err != nil {
		apiErr(&resp.Diagnostics, "Deleting bridge", err)
	}
}

func (r *bridgeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nodeID, name, ok := strings.Cut(req.ID, "/")
	if !ok || nodeID == "" || name == "" {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <node_id>/<bridge_name>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("node_id"), nodeID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
