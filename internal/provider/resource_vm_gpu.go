package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = (*vmGPUResource)(nil)
	_ resource.ResourceWithImportState = (*vmGPUResource)(nil)
)

type vmGPUResource struct{ base }

func newVMGPUResource() resource.Resource { return &vmGPUResource{} }

type vmGPUModel struct {
	ID       types.String   `tfsdk:"id"`
	VMID     types.String   `tfsdk:"vm_id"`
	Address  types.String   `tfsdk:"address"`
	Devices  types.List     `tfsdk:"devices"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

var pciAddressRe = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)

func (r *vmGPUResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_gpu"
}

func (r *vmGPUResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Passes a GPU PCI slot, or a pre-provisioned SR-IOV virtual function, through to a VM. " +
			"The VM must be a stopped Q35/UEFI VM without snapshots, so set `power_state = \"stopped\"` on the VM while " +
			"attaching or detaching. The device's IOMMU group must already be bound to `vfio-pci`.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"vm_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"address": schema.StringAttribute{
				Required: true, PlanModifiers: replace, MarkdownDescription: "PCI address, e.g. `0000:01:00.0`.",
				Validators: []validator.String{stringvalidator.RegexMatches(pciAddressRe, "must be a lowercase PCI address like 0000:01:00.0")},
			},
			"devices": schema.ListAttribute{
				ElementType: types.StringType, Computed: true,
				MarkdownDescription: "PCI functions assigned with the GPU (its IOMMU group members).",
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

func (r *vmGPUResource) set(ctx context.Context, m *vmGPUModel, attach bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	task, err := r.client.SetVMGPU(ctx, m.VMID.ValueString(), m.Address.ValueString(), attach)
	if err != nil {
		return err
	}
	_, err = r.client.WaitTask(ctx, task.ID)
	return err
}

func (r *vmGPUResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmGPUModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := plan.Timeouts.Create(ctx, 15*time.Minute)
	resp.Diagnostics.Append(d...)
	if err := r.set(ctx, &plan, true, timeout); err != nil {
		apiErr(&resp.Diagnostics, "Attaching GPU", err)
		return
	}
	plan.ID = types.StringValue(plan.VMID.ValueString() + "/" + plan.Address.ValueString())
	plan.Devices = types.ListNull(types.StringType)
	if vm, err := r.client.GetVM(ctx, plan.VMID.ValueString()); err == nil && vm.Spec.GPU != nil {
		plan.Devices = stringList(ctx, vm.Spec.GPU.Devices, &resp.Diagnostics)
	}
	if plan.Devices.IsNull() {
		plan.Devices = stringList(ctx, nil, &resp.Diagnostics)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmGPUResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmGPUModel
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
	if vm.Spec.GPU == nil || vm.Spec.GPU.Address != state.Address.ValueString() {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Devices = stringList(ctx, vm.Spec.GPU.Devices, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vmGPUResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vmGPUModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmGPUResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmGPUModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, d := state.Timeouts.Delete(ctx, 15*time.Minute)
	resp.Diagnostics.Append(d...)
	if err := r.set(ctx, &state, false, timeout); err != nil && !client.IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Detaching GPU", err)
	}
}

func (r *vmGPUResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vmID, addr, ok := strings.Cut(req.ID, "/")
	if !ok || vmID == "" || addr == "" {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <vm_id>/<pci_address>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("address"), addr)...)
}
