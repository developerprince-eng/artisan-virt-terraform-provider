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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*vmSnapshotResource)(nil)
	_ resource.ResourceWithImportState = (*vmSnapshotResource)(nil)
)

type vmSnapshotResource struct{ base }

func newVMSnapshotResource() resource.Resource { return &vmSnapshotResource{} }

type vmSnapshotModel struct {
	ID          types.String   `tfsdk:"id"`
	VMID        types.String   `tfsdk:"vm_id"`
	Name        types.String   `tfsdk:"name"`
	Description types.String   `tfsdk:"description"`
	CreatedAt   types.String   `tfsdk:"created_at"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func (r *vmSnapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_snapshot"
}

func (r *vmSnapshotResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An internal libvirt snapshot of a VM. Detach any GPU first; VMs with snapshots cannot get a GPU.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: keep, MarkdownDescription: "`<vm_id>/<name>`."},
			"vm_id":       schema.StringAttribute{Required: true, PlanModifiers: replace},
			"name":        schema.StringAttribute{Required: true, PlanModifiers: replace, Validators: []validator.String{nameValidator()}},
			"description": schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"created_at":  schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

func findSnapshot(vm *client.VM, name string) *client.Snapshot {
	for i := range vm.Snapshots {
		if vm.Snapshots[i].Name == name {
			return &vm.Snapshots[i]
		}
	}
	return nil
}

func (r *vmSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmSnapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 20*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	vmID, name := plan.VMID.ValueString(), plan.Name.ValueString()
	task, err := r.client.CreateSnapshot(ctx, vmID, client.SnapshotRequest{Name: name, Description: plan.Description.ValueString()})
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating snapshot "+name, err)
		return
	}
	if _, err := r.client.WaitTask(ctx, task.ID); err != nil {
		apiErr(&resp.Diagnostics, "Creating snapshot "+name, err)
		return
	}
	plan.ID = types.StringValue(vmID + "/" + name)
	plan.CreatedAt = types.StringNull()
	if vm, err := r.client.GetVM(ctx, vmID); err == nil {
		if s := findSnapshot(vm, name); s != nil {
			plan.CreatedAt = types.StringValue(s.CreatedAt.UTC().Format(time.RFC3339))
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmSnapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vm, err := r.client.GetVM(ctx, state.VMID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading VM", err)
		return
	}
	s := findSnapshot(vm, state.Name.ValueString())
	if s == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Description = keepNullString(state.Description, s.Description)
	state.CreatedAt = types.StringValue(s.CreatedAt.UTC().Format(time.RFC3339))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vmSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vmSnapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmSnapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 20*time.Minute)
	resp.Diagnostics.Append(d...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	task, err := r.client.DeleteSnapshot(ctx, state.VMID.ValueString(), state.Name.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Deleting snapshot", err)
		return
	}
	if _, err := r.client.WaitTask(ctx, task.ID); err != nil {
		apiErr(&resp.Diagnostics, "Deleting snapshot", err)
	}
}

func (r *vmSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vmID, name, ok := strings.Cut(req.ID, "/")
	if !ok || vmID == "" || name == "" {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <vm_id>/<snapshot_name>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}
