package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- artisanvirt_backup_targets ----------------------------------------------

type backupTargetsDataSource struct{ dsBase }

func newBackupTargetsDataSource() datasource.DataSource { return &backupTargetsDataSource{} }

type backupTargetData struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Path        types.String `tfsdk:"path"`
	NFSServer   types.String `tfsdk:"nfs_server"`
	NFSExport   types.String `tfsdk:"nfs_export"`
	ClusterIDs  types.List   `tfsdk:"cluster_ids"`
	Description types.String `tfsdk:"description"`
}

var backupTargetAttrTypes = map[string]attr.Type{
	"id": types.StringType, "name": types.StringType, "type": types.StringType, "path": types.StringType,
	"nfs_server": types.StringType, "nfs_export": types.StringType,
	"cluster_ids": types.ListType{ElemType: types.StringType}, "description": types.StringType,
}

type backupTargetsModel struct {
	Targets types.List `tfsdk:"targets"`
}

func (d *backupTargetsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_targets"
}

func (d *backupTargetsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := schema.StringAttribute{Computed: true}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists backup targets.",
		Attributes: map[string]schema.Attribute{
			"targets": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": str, "name": str, "type": str, "path": str, "nfs_server": str, "nfs_export": str, "description": str,
					"cluster_ids": schema.ListAttribute{Computed: true, ElementType: types.StringType},
				},
			}},
		},
	}
}

func (d *backupTargetsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	ts, err := d.client.ListBackupTargets(ctx)
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing backup targets", err)
		return
	}
	out := make([]backupTargetData, 0, len(ts))
	for _, t := range ts {
		out = append(out, backupTargetData{
			ID: types.StringValue(t.ID), Name: types.StringValue(t.Name), Type: types.StringValue(t.Type),
			Path: types.StringValue(t.Path), NFSServer: types.StringValue(t.NFSServer), NFSExport: types.StringValue(t.NFSExport),
			ClusterIDs: stringList(ctx, t.ClusterIDs, &resp.Diagnostics), Description: types.StringValue(t.Description),
		})
	}
	m := backupTargetsModel{Targets: objectList(ctx, backupTargetAttrTypes, out, &resp.Diagnostics)}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// --- artisanvirt_backups -----------------------------------------------------

type backupsDataSource struct{ dsBase }

func newBackupsDataSource() datasource.DataSource { return &backupsDataSource{} }

type backupData struct {
	ID          types.String `tfsdk:"id"`
	VMID        types.String `tfsdk:"vm_id"`
	VMName      types.String `tfsdk:"vm_name"`
	NodeID      types.String `tfsdk:"node_id"`
	TargetID    types.String `tfsdk:"target_id"`
	PolicyID    types.String `tfsdk:"policy_id"`
	Status      types.String `tfsdk:"status"`
	SizeBytes   types.Int64  `tfsdk:"size_bytes"`
	CreatedAt   types.String `tfsdk:"created_at"`
	CompletedAt types.String `tfsdk:"completed_at"`
}

var backupAttrTypes = map[string]attr.Type{
	"id": types.StringType, "vm_id": types.StringType, "vm_name": types.StringType, "node_id": types.StringType,
	"target_id": types.StringType, "policy_id": types.StringType, "status": types.StringType,
	"size_bytes": types.Int64Type, "created_at": types.StringType, "completed_at": types.StringType,
}

type backupsModel struct {
	VMID     types.String `tfsdk:"vm_id"`
	TargetID types.String `tfsdk:"target_id"`
	Backups  types.List   `tfsdk:"backups"`
}

func (d *backupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backups"
}

func (d *backupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := schema.StringAttribute{Computed: true}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists backups, newest first, optionally filtered by VM or target.",
		Attributes: map[string]schema.Attribute{
			"vm_id":     schema.StringAttribute{Optional: true},
			"target_id": schema.StringAttribute{Optional: true},
			"backups": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": str, "vm_id": str, "vm_name": str, "node_id": str, "target_id": str, "policy_id": str,
					"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "`queued`, `running`, `completed`, `failed` or `pruning`."},
					"size_bytes": schema.Int64Attribute{Computed: true}, "created_at": str, "completed_at": str,
				},
			}},
		},
	}
}

func (d *backupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg backupsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	bs, err := d.client.ListBackups(ctx, cfg.VMID.ValueString(), cfg.TargetID.ValueString())
	if err != nil {
		apiErr(&resp.Diagnostics, "Listing backups", err)
		return
	}
	out := make([]backupData, 0, len(bs))
	for _, b := range bs {
		out = append(out, backupData{
			ID: types.StringValue(b.ID), VMID: types.StringValue(b.VMID), VMName: types.StringValue(b.VMName),
			NodeID: types.StringValue(b.NodeID), TargetID: types.StringValue(b.TargetID), PolicyID: types.StringValue(b.PolicyID),
			Status: types.StringValue(b.Status), SizeBytes: types.Int64Value(int64(b.SizeBytes)),
			CreatedAt: types.StringValue(b.CreatedAt.UTC().Format(time.RFC3339)), CompletedAt: timeString(b.CompletedAt),
		})
	}
	cfg.Backups = objectList(ctx, backupAttrTypes, out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
