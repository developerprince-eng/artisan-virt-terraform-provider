package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*backupPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*backupPolicyResource)(nil)
)

type backupPolicyResource struct{ base }

func newBackupPolicyResource() resource.Resource { return &backupPolicyResource{} }

type backupPolicyModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Enabled   types.Bool   `tfsdk:"enabled"`
	Schedule  types.String `tfsdk:"schedule"`
	TargetID  types.String `tfsdk:"target_id"`
	KeepLast  types.Int64  `tfsdk:"keep_last"`
	Selector  types.Object `tfsdk:"selector"`
	LastRunAt types.String `tfsdk:"last_run_at"`
	NextRunAt types.String `tfsdk:"next_run_at"`
}

type selectorModel struct {
	VMIDs      types.Set `tfsdk:"vm_ids"`
	ClusterIDs types.Set `tfsdk:"cluster_ids"`
	Tags       types.Map `tfsdk:"tags"`
}

var selectorAttrTypes = map[string]attr.Type{
	"vm_ids":      types.SetType{ElemType: types.StringType},
	"cluster_ids": types.SetType{ElemType: types.StringType},
	"tags":        types.MapType{ElemType: types.StringType},
}

func (r *backupPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_policy"
}

func (r *backupPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A scheduled backup policy. A VM matches the selector if it is listed in `vm_ids`, is in one of " +
			"`cluster_ids`, or carries all `tags`. An empty selector matches nothing. Deleting a policy keeps its backups.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, Validators: []validator.String{nameValidator()}},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether the schedule runs. Defaults to `true`.",
			},
			"schedule": schema.StringAttribute{
				Required: true, MarkdownDescription: "Standard 5-field cron expression, e.g. `0 2 * * *`.",
			},
			"target_id": schema.StringAttribute{Required: true, MarkdownDescription: "Backup target id."},
			"keep_last": schema.Int64Attribute{
				Required: true, Validators: []validator.Int64{int64validator.Between(1, 1000)},
				MarkdownDescription: "Completed backups to keep per VM.",
			},
			"selector": schema.SingleNestedAttribute{
				Required: true, MarkdownDescription: "Which VMs the policy backs up.",
				Attributes: map[string]schema.Attribute{
					"vm_ids":      schema.SetAttribute{ElementType: types.StringType, Optional: true},
					"cluster_ids": schema.SetAttribute{ElementType: types.StringType, Optional: true},
					"tags":        schema.MapAttribute{ElementType: types.StringType, Optional: true},
				},
			},
			"last_run_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Last run (RFC 3339)."},
			"next_run_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Next scheduled run (RFC 3339)."},
		},
	}
}

func (r *backupPolicyResource) request(ctx context.Context, m *backupPolicyModel, diags *diag.Diagnostics) client.PolicyRequest {
	var sel selectorModel
	diags.Append(m.Selector.As(ctx, &sel, basetypes.ObjectAsOptions{})...)
	return client.PolicyRequest{
		Name:     m.Name.ValueString(),
		Enabled:  m.Enabled.ValueBool(),
		Schedule: m.Schedule.ValueString(),
		TargetID: m.TargetID.ValueString(),
		KeepLast: int(m.KeepLast.ValueInt64()),
		Selector: client.BackupSelector{
			VMIDs:      toStrings(ctx, sel.VMIDs, diags),
			ClusterIDs: toStrings(ctx, sel.ClusterIDs, diags),
			Tags:       toStringMap(ctx, sel.Tags, diags),
		},
	}
}

func setPolicyState(ctx context.Context, m *backupPolicyModel, p *client.BackupPolicy, diags *diag.Diagnostics) {
	var prior selectorModel
	if !m.Selector.IsNull() && !m.Selector.IsUnknown() {
		diags.Append(m.Selector.As(ctx, &prior, basetypes.ObjectAsOptions{})...)
	} else {
		prior = selectorModel{
			VMIDs: types.SetNull(types.StringType), ClusterIDs: types.SetNull(types.StringType), Tags: types.MapNull(types.StringType),
		}
	}
	sel := selectorModel{
		VMIDs:      keepNullSet(prior.VMIDs, stringSet(ctx, p.Selector.VMIDs, diags)),
		ClusterIDs: keepNullSet(prior.ClusterIDs, stringSet(ctx, p.Selector.ClusterIDs, diags)),
		Tags:       keepNullMap(prior.Tags, stringMap(ctx, p.Selector.Tags, diags)),
	}
	obj, d := types.ObjectValueFrom(ctx, selectorAttrTypes, sel)
	diags.Append(d...)

	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Enabled = types.BoolValue(p.Enabled)
	m.Schedule = types.StringValue(p.Schedule)
	m.TargetID = types.StringValue(p.TargetID)
	m.KeepLast = types.Int64Value(int64(p.KeepLast))
	m.Selector = obj
	m.LastRunAt = timeString(p.LastRunAt)
	m.NextRunAt = timeString(p.NextRunAt)
}

func (r *backupPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan backupPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.request(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.CreateBackupPolicy(ctx, body)
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating backup policy", err)
		return
	}
	setPolicyState(ctx, &plan, p, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state backupPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.GetBackupPolicy(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading backup policy", err)
		return
	}
	setPolicyState(ctx, &state, p, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *backupPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state backupPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.request(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.UpdateBackupPolicy(ctx, state.ID.ValueString(), body)
	if err != nil {
		apiErr(&resp.Diagnostics, "Updating backup policy", err)
		return
	}
	setPolicyState(ctx, &plan, p, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state backupPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBackupPolicy(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Deleting backup policy", err)
	}
}

func (r *backupPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
