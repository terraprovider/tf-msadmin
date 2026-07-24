package genframework

import (
	"strings"
	"testing"
)

// TestMemberCollectionWiring checks that a resource with a members sub-collection
// generates the read/update/reconcile plumbing and a retrying member write.
func TestMemberCollectionWiring(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "EXO",
		BindingsImport: "github.com/terraprovider/go-exoscc/exo", BindingsPkg: "exo",
	}
	r := Resource{
		Noun: "RoleGroup", TFName: "role_group", Description: "A role group.",
		Attributes: []Attribute{
			{TFName: "name", Field: "Name", APIName: "Name", Type: TypeString, Required: true, Replace: true, Description: "Name.", InCreate: true},
		},
		Create: Op{Method: "NewRoleGroup", Params: "NewRoleGroupParams"},
		Read:   Op{Method: "GetRoleGroup", Params: "GetRoleGroupParams", IdentityField: "Identity"},
		Update: Op{Method: "SetRoleGroup", Params: "SetRoleGroupParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveRoleGroup", Params: "RemoveRoleGroupParams", IdentityField: "Identity"},
		Members: &MemberCollection{
			TFName: "members", Field: "Members", Description: "Members.",
			ReadMethod: "GetRoleGroupMember", ReadParams: "GetRoleGroupMemberParams",
			UpdateMethod: "UpdateRoleGroupMember", UpdateParams: "UpdateRoleGroupMemberParams",
			IdentityField: "Identity", MembersField: "Members",
			ReadKeys: []string{"PrimarySmtpAddress", "Name"},
		},
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res, ds string
	for _, f := range files {
		switch f.Name {
		case "role_group_resource.go":
			res = string(f.Content)
		case "role_group_data_source.go":
			ds = string(f.Content)
		}
	}
	for _, want := range []string{
		"`tfsdk:\"members\"`",
		"func readRoleGroupMembers(ctx context.Context, svc *exo.Service",
		"svc.GetRoleGroupMember(ctx, exo.GetRoleGroupMemberParams{Identity: identity})",
		"getString(mm, \"PrimarySmtpAddress\")",
		"resourcex.RetryWrite(ctx, consistency.Config{}",
		"UpdateRoleGroupMember(ctx, exo.UpdateRoleGroupMemberParams{Identity: ident, Members: mem})",
		"reconcile.KeepSet(cfg.Members, read.Members)",
	} {
		if !strings.Contains(res, want) {
			t.Errorf("resource missing %q", want)
		}
	}
	if !strings.Contains(ds, "readRoleGroupMembers(ctx, d.client.EXO, identity, &data)") {
		t.Error("data source does not read members")
	}
}
