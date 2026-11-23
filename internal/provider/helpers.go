package provider

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

// base is embedded by every resource and data source to receive the client.
type base struct {
	client *client.Client
}

func (b *base) configure(data any, diags *diag.Diagnostics) {
	if data == nil {
		return // provider not configured yet (validation-only RPCs)
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", data))
		return
	}
	b.client = c
}

func (b *base) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	b.configure(req.ProviderData, &resp.Diagnostics)
}

type dsBase struct{ base }

func (b *dsBase) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	b.configure(req.ProviderData, &resp.Diagnostics)
}

func stringOr(v types.String, fallback string) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return fallback
	}
	return v.ValueString()
}

func int64Or(v types.Int64, fallback int64) int64 {
	if v.IsNull() || v.IsUnknown() || v.ValueInt64() <= 0 {
		return fallback
	}
	return v.ValueInt64()
}

// optString maps "" to null so optional attributes the user left out stay null.
func optString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func timeString(t *time.Time) types.String {
	if t == nil || t.IsZero() {
		return types.StringNull()
	}
	return types.StringValue(t.UTC().Format(time.RFC3339))
}

func stringList(ctx context.Context, in []string, diags *diag.Diagnostics) types.List {
	if in == nil {
		in = []string{}
	}
	l, d := types.ListValueFrom(ctx, types.StringType, in)
	diags.Append(d...)
	return l
}

func stringSet(ctx context.Context, in []string, diags *diag.Diagnostics) types.Set {
	if in == nil {
		in = []string{}
	}
	s, d := types.SetValueFrom(ctx, types.StringType, in)
	diags.Append(d...)
	return s
}

func stringMap(ctx context.Context, in map[string]string, diags *diag.Diagnostics) types.Map {
	if in == nil {
		in = map[string]string{}
	}
	m, d := types.MapValueFrom(ctx, types.StringType, in)
	diags.Append(d...)
	return m
}

// keepNullWhenEmpty returns null when the API value is empty and the prior
// value was null, so an omitted optional collection does not show a diff.
func keepNullMap(prior types.Map, got types.Map) types.Map {
	if (prior.IsNull() || prior.IsUnknown()) && len(got.Elements()) == 0 {
		return types.MapNull(types.StringType)
	}
	return got
}

func keepNullSet(prior types.Set, got types.Set) types.Set {
	if (prior.IsNull() || prior.IsUnknown()) && len(got.Elements()) == 0 {
		return types.SetNull(types.StringType)
	}
	return got
}

func toStrings(ctx context.Context, v interface {
	ElementsAs(context.Context, any, bool) diag.Diagnostics
	IsNull() bool
	IsUnknown() bool
}, diags *diag.Diagnostics) []string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	return out
}

func toStringMap(ctx context.Context, v types.Map, diags *diag.Diagnostics) map[string]string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := map[string]string{}
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	return out
}

func objectList(ctx context.Context, elemType map[string]attr.Type, in any, diags *diag.Diagnostics) types.List {
	l, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: elemType}, in)
	diags.Append(d...)
	return l
}

func apiErr(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, err.Error())
}

// nameRe mirrors protocol.ValidName on the server: names end up in libvirt XML
// and file paths.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

func nameValidator() validator.String {
	return stringvalidator.RegexMatches(nameRe,
		"must start with a letter or digit and contain at most 63 letters, digits, '_', '.' or '-'")
}
