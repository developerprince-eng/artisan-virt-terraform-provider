// Package provider implements the Artisan Virt Terraform provider.
package provider

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt/internal/client"
)

// RegistryAddress is the provider source address on the Terraform Registry.
const RegistryAddress = "registry.terraform.io/developerprince/artisanvirt"

var _ provider.Provider = (*artisanProvider)(nil)

type artisanProvider struct {
	version string
}

// New returns a provider factory for providerserver.Serve.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &artisanProvider{version: version} }
}

type providerModel struct {
	Endpoint            types.String `tfsdk:"endpoint"`
	Token               types.String `tfsdk:"token"`
	Insecure            types.Bool   `tfsdk:"insecure"`
	CACert              types.String `tfsdk:"ca_cert"`
	RequestTimeout      types.Int64  `tfsdk:"request_timeout"`
	PollIntervalSeconds types.Int64  `tfsdk:"poll_interval"`
}

func (p *artisanProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "artisanvirt"
	resp.Version = p.version
}

func (p *artisanProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Artisan Virt provider manages hypervisor nodes, clusters, virtual machines, " +
			"host bridges and backups through the Artisan Virt control plane REST API (`/api/v1`).",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Base URL of the control plane, for example `https://artisan.example.com`. " +
					"Can also be set with the `ARTISANVIRT_ENDPOINT` environment variable.",
				Optional: true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Administrator bearer token. Can also be set with the `ARTISANVIRT_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification (lab installs only). Env: `ARTISANVIRT_INSECURE`.",
				Optional:            true,
			},
			"ca_cert": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded CA certificate trusted in addition to the system roots. Env: `ARTISANVIRT_CA_CERT`.",
				Optional:            true,
			},
			"request_timeout": schema.Int64Attribute{
				MarkdownDescription: "Timeout in seconds for a single HTTP request. Defaults to `60`.",
				Optional:            true,
			},
			"poll_interval": schema.Int64Attribute{
				MarkdownDescription: "Seconds between polls while waiting for asynchronous tasks. Defaults to `3`.",
				Optional:            true,
			},
		},
	}
}

func (p *artisanProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Endpoint.IsUnknown() || cfg.Token.IsUnknown() {
		resp.Diagnostics.AddWarning("Provider configuration is not yet known",
			"endpoint or token depends on values known only after apply; the provider cannot connect during this plan.")
		return
	}

	endpoint := stringOr(cfg.Endpoint, os.Getenv("ARTISANVIRT_ENDPOINT"))
	token := stringOr(cfg.Token, os.Getenv("ARTISANVIRT_TOKEN"))
	caCert := stringOr(cfg.CACert, os.Getenv("ARTISANVIRT_CA_CERT"))
	insecure := false
	if !cfg.Insecure.IsNull() {
		insecure = cfg.Insecure.ValueBool()
	} else if v, err := strconv.ParseBool(os.Getenv("ARTISANVIRT_INSECURE")); err == nil {
		insecure = v
	}

	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Missing Artisan Virt endpoint",
			"Set the endpoint provider argument or the ARTISANVIRT_ENDPOINT environment variable.")
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Missing Artisan Virt token",
			"Set the token provider argument or the ARTISANVIRT_TOKEN environment variable.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.New(client.Config{
		Endpoint:           endpoint,
		Token:              token,
		InsecureSkipVerify: insecure,
		CACertPEM:          caCert,
		RequestTimeout:     time.Duration(int64Or(cfg.RequestTimeout, 60)) * time.Second,
		PollInterval:       time.Duration(int64Or(cfg.PollIntervalSeconds, 3)) * time.Second,
		UserAgent:          "terraform-provider-artisanvirt/" + p.version,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Artisan Virt client", err.Error())
		return
	}
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *artisanProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newEnrollmentTokenResource,
		newNodeResource,
		newBridgeResource,
		newClusterResource,
		newClusterMemberResource,
		newVMResource,
		newVMSnapshotResource,
		newVMGPUResource,
		newBackupTargetResource,
		newBackupPolicyResource,
	}
}

func (p *artisanProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newNodeDataSource,
		newNodesDataSource,
		newClusterDataSource,
		newClustersDataSource,
		newVMDataSource,
		newVMsDataSource,
		newBackupTargetsDataSource,
		newBackupsDataSource,
	}
}
