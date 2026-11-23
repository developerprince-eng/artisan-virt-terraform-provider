package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*backupTargetResource)(nil)
	_ resource.ResourceWithImportState = (*backupTargetResource)(nil)
)

type backupTargetResource struct{ base }

func newBackupTargetResource() resource.Resource { return &backupTargetResource{} }

type backupTargetModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Type                   types.String `tfsdk:"type"`
	Path                   types.String `tfsdk:"path"`
	NFSServer              types.String `tfsdk:"nfs_server"`
	NFSExport              types.String `tfsdk:"nfs_export"`
	ClusterIDs             types.Set    `tfsdk:"cluster_ids"`
	Description            types.String `tfsdk:"description"`
	ForgetBackupsOnDestroy types.Bool   `tfsdk:"forget_backups_on_destroy"`
	CreatedAt              types.String `tfsdk:"created_at"`
}

func (r *backupTargetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_target"
}

func (r *backupTargetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A backup repository. `nfs` targets are mounted on every node of the attached clusters, " +
			"which allows restoring into another cluster; `filesystem` targets are a node-local directory. " +
			"Targets cannot be changed in place.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, PlanModifiers: replace, Validators: []validator.String{nameValidator()}},
			"type": schema.StringAttribute{
				Required: true, PlanModifiers: replace, MarkdownDescription: "`nfs` or `filesystem`.",
				Validators: []validator.String{stringvalidator.OneOf("nfs", "filesystem")},
			},
			"path": schema.StringAttribute{
				Required: true, PlanModifiers: replace,
				MarkdownDescription: "Absolute directory (mount point for NFS) on the nodes.",
			},
			"nfs_server": schema.StringAttribute{Optional: true, PlanModifiers: replace, MarkdownDescription: "NFS server (type `nfs`)."},
			"nfs_export": schema.StringAttribute{Optional: true, PlanModifiers: replace, MarkdownDescription: "NFS export path (type `nfs`)."},
			"cluster_ids": schema.SetAttribute{
				ElementType: types.StringType, Optional: true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
				MarkdownDescription: "Clusters whose nodes can reach this target.",
			},
			"description": schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"forget_backups_on_destroy": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "On destroy, drop the records of backups still on the target (the files are left on the " +
					"target). Without it, destroying a target that holds backups fails.",
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *backupTargetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan backupTargetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.CreateBackupTarget(ctx, client.CreateTargetRequest{
		Name:        plan.Name.ValueString(),
		Type:        plan.Type.ValueString(),
		Path:        plan.Path.ValueString(),
		NFSServer:   plan.NFSServer.ValueString(),
		NFSExport:   plan.NFSExport.ValueString(),
		ClusterIDs:  toStrings(ctx, plan.ClusterIDs, &resp.Diagnostics),
		Description: plan.Description.ValueString(),
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating backup target", err)
		return
	}
	plan.ID = types.StringValue(t.ID)
	plan.CreatedAt = types.StringValue(t.CreatedAt.UTC().Format(time.RFC3339))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupTargetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state backupTargetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetBackupTarget(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading backup target", err)
		return
	}
	state.Name = types.StringValue(t.Name)
	state.Type = types.StringValue(t.Type)
	state.Path = types.StringValue(t.Path)
	state.NFSServer = keepNullString(state.NFSServer, t.NFSServer)
	state.NFSExport = keepNullString(state.NFSExport, t.NFSExport)
	state.Description = keepNullString(state.Description, t.Description)
	// Destroying a cluster detaches it from targets server-side; that is drift
	// Terraform should show, so cluster_ids is refreshed from the API.
	state.ClusterIDs = keepNullSet(state.ClusterIDs, stringSet(ctx, t.ClusterIDs, &resp.Diagnostics))
	state.CreatedAt = types.StringValue(t.CreatedAt.UTC().Format(time.RFC3339))
	if state.ForgetBackupsOnDestroy.IsNull() {
		state.ForgetBackupsOnDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *backupTargetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan backupTargetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupTargetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state backupTargetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteBackupTarget(ctx, state.ID.ValueString(), state.ForgetBackupsOnDestroy.ValueBool())
	if err != nil && !client.IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Deleting backup target", err)
	}
}

func (r *backupTargetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
