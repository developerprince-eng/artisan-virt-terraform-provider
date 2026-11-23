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

type vmData struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	NodeID      types.String `tfsdk:"node_id"`
	State       types.String `tfsdk:"state"`
	LastError   types.String `tfsdk:"last_error"`
	UUID        types.String `tfsdk:"uuid"`
	VCPUs       types.Int64  `tfsdk:"vcpus"`
	MemoryMiB   types.Int64  `tfsdk:"memory_mib"`
	Firmware    types.String `tfsdk:"firmware"`
	Autostart   types.Bool   `tfsdk:"autostart"`
	MACs        types.List   `tfsdk:"mac_addresses"`
	Snapshots   types.List   `tfsdk:"snapshots"`
	Tags        types.Map    `tfsdk:"tags"`
	GPUAddress  types.String `tfsdk:"gpu_address"`
}

var vmDataAttrTypes = map[string]attr.Type{
	"id": types.StringType, "name": types.StringType, "description": types.StringType, "cluster_id": types.StringType,
	"node_id": types.StringType, "state": types.StringType, "last_error": types.StringType, "uuid": types.StringType,
	"vcpus": types.Int64Type, "memory_mib": types.Int64Type, "firmware": types.StringType, "autostart": types.BoolType,
	"mac_addresses": types.ListType{ElemType: types.StringType}, "snapshots": types.ListType{ElemType: types.StringType},
	"tags": types.MapType{ElemType: types.StringType}, "gpu_address": types.StringType,
}

func toVMData(ctx context.Context, vm *client.VM, diags *diag.Diagnostics) vmData {
	macs := make([]string, 0, len(vm.Spec.NICs))
	for _, n := range vm.Spec.NICs {
		macs = append(macs, n.MAC)
	}
	snaps := make([]string, 0, len(vm.Snapshots))
	for _, s := range vm.Snapshots {
		snaps = append(snaps, s.Name)
	}
	gpu := ""
	if vm.Spec.GPU != nil {
		gpu = vm.Spec.GPU.Address
	}
	return vmData{
		ID: types.StringValue(vm.ID), Name: types.StringValue(vm.Name), Description: types.StringValue(vm.Description),
		ClusterID: types.StringValue(vm.ClusterID), NodeID: types.StringValue(vm.NodeID), State: types.StringValue(vm.State),
		LastError: types.StringValue(vm.LastError), UUID: types.StringValue(vm.Spec.UUID),
		VCPUs: types.Int64Value(int64(vm.Spec.VCPUs)), MemoryMiB: types.Int64Value(int64(vm.Spec.MemoryMiB)),
		Firmware: types.StringValue(vm.Spec.Firmware), Autostart: types.BoolValue(vm.Spec.Autostart),
		MACs: stringList(ctx, macs, diags), Snapshots: stringList(ctx, snaps, diags),
		Tags: stringMap(ctx, vm.Tags, diags), GPUAddress: types.StringValue(gpu),
	}
}

func vmSchemaAttrs(lookup bool) map[string]schema.Attribute {
	str := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	attrs := map[string]schema.Attribute{
		"id": str("VM id."), "name": str("VM name."), "description": str(""), "cluster_id": str(""), "node_id": str("Host node."),
		"state": str("VM state."), "last_error": str(""), "uuid": str("libvirt UUID."),
		"vcpus":         schema.Int64Attribute{Computed: true},
		"memory_mib":    schema.Int64Attribute{Computed: true},
		"firmware":      str("`bios` or `uefi`."),
		"autostart":     schema.BoolAttribute{Computed: true},
		"mac_addresses": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "NIC MACs in order."},
		"snapshots":     schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Snapshot names."},
		"tags":          schema.MapAttribute{Computed: true, ElementType: types.StringType},
		"gpu_address":   str("Assigned GPU PCI address, if any."),
	}
	if lookup {
		attrs["id"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "VM id. Exactly one of `id` or `name` is required."}
		attrs["name"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "VM name."}
	}
	return attrs
}

// --- artisanvirt_vm ----------------------------------------------------------

type vmDataSource struct{ dsBase }

func newVMDataSource() datasource.DataSource { return &vmDataSource{} }

func (d *vmDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (d *vmDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Looks up a VM by id or name.", Attributes: vmSchemaAttrs(true)}
}

func (d *vmDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *vmDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg vmData
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.VM
	if id := cfg.ID.ValueString(); id != "" {
		vm, err := d.client.GetVM(ctx, id)
		if err != nil {
			apiErr(&resp.Diagnostics, "Reading VM "+id, err)
			return
		}
		found = vm
	} else {
		vms, err := d.client.ListVMs(ctx, "", "")
		if err != nil {
			apiErr(&resp.Diagnostics, "Listing VMs", err)
			return
		}
		for _, vm := range vms {
			if vm.Name == cfg.Name.ValueString() {
				found = vm
			}
		}
		if found == nil {
			resp.Diagnostics.AddError("VM not found", fmt.Sprintf("no VM named %q", cfg.Name.ValueString()))
			return
		}
	}
	out := toVMData(ctx, found, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}

// --- artisanvirt_vms ---------------------------------------------------------

type vmsDataSource struct{ dsBase }

func newVMsDataSource() datasource.DataSource { return &vmsDataSource{} }

type vmsModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	NodeID    types.String `tfsdk:"node_id"`
	VMs       types.List   `tfsdk:"vms"`
}

func (d *vmsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vms"
}

func (d *vmsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists VMs, optionally filtered by cluster or node.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Optional: true},
			"node_id":    schema.StringAttribute{Optional: true},
			"vms": schema.ListNestedAttribute{
				Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: vmSchemaAttrs(false)},
			},
		},
	}
}

func (d *vmsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg vmsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vms, err := d.client.ListVMs(ctx, cfg.ClusterID.ValueString(), cfg.NodeID.ValueString())
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing VMs", err)
		return
	}
	out := make([]vmData, 0, len(vms))
	for _, vm := range vms {
		out = append(out, toVMData(ctx, vm, &resp.Diagnostics))
	}
	cfg.VMs = objectList(ctx, vmDataAttrTypes, out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
