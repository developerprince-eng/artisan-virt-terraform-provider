package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

var (
	_ resource.Resource                = (*enrollmentTokenResource)(nil)
	_ resource.ResourceWithImportState = (*enrollmentTokenResource)(nil)
)

type enrollmentTokenResource struct{ base }

func newEnrollmentTokenResource() resource.Resource { return &enrollmentTokenResource{} }

type enrollmentTokenModel struct {
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	TTLMinutes  types.Int64  `tfsdk:"ttl_minutes"`
	Value       types.String `tfsdk:"value"`
	ExpiresAt   types.String `tfsdk:"expires_at"`
	UsedAt      types.String `tfsdk:"used_at"`
	UsedByNode  types.String `tfsdk:"used_by_node"`
}

func (r *enrollmentTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_enrollment_token"
}

func (r *enrollmentTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A single-use enrollment token. Pass `value` to the hypervisor agent so it can enroll the host; " +
			"the node then appears as `discovered` and can be approved with `artisanvirt_node`. " +
			"The secret is returned only once, at creation, and is stored in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Token id.", PlanModifiers: keep},
			"description": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Free-text description.", PlanModifiers: replace,
			},
			"ttl_minutes": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Lifetime in minutes (max 1440). Defaults to the server setting.",
				Validators:    []validator.Int64{int64validator.Between(1, 1440)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"value": schema.StringAttribute{
				Computed: true, Sensitive: true, PlanModifiers: keep,
				MarkdownDescription: "The token secret (`avt_...`) to give to the agent.",
			},
			"expires_at":   schema.StringAttribute{Computed: true, PlanModifiers: keep, MarkdownDescription: "Expiry time (RFC 3339)."},
			"used_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "When an agent used the token, if it has."},
			"used_by_node": schema.StringAttribute{Computed: true, MarkdownDescription: "Node id that used the token, if any."},
		},
	}
}

func (r *enrollmentTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan enrollmentTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.CreateEnrollmentToken(ctx, client.CreateTokenRequest{
		Description: plan.Description.ValueString(),
		TTLMinutes:  int(plan.TTLMinutes.ValueInt64()),
	})
	if err != nil {
		apiErr(&resp.Diagnostics, "Creating enrollment token", err)
		return
	}
	plan.Value = types.StringValue(out.Value)
	setTokenState(&plan, out.Token)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func setTokenState(m *enrollmentTokenModel, t *client.EnrollmentToken) {
	m.ID = types.StringValue(t.ID)
	if t.Description != "" || !m.Description.IsNull() {
		m.Description = types.StringValue(t.Description)
	}
	m.ExpiresAt = types.StringValue(t.ExpiresAt.UTC().Format(time.RFC3339))
	m.UsedAt = timeString(t.UsedAt)
	m.UsedByNode = optString(t.UsedByNode)
}

func (r *enrollmentTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state enrollmentTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ts, err := r.client.ListEnrollmentTokens(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Reading enrollment tokens", err)
		return
	}
	for _, t := range ts {
		if t.ID == state.ID.ValueString() {
			setTokenState(&state, t)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *enrollmentTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every configurable attribute forces replacement.
	var plan enrollmentTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *enrollmentTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state enrollmentTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RevokeEnrollmentToken(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Revoking enrollment token", err)
	}
}

func (r *enrollmentTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
