package genframework

import (
	"strings"
	"testing"
)

func roleGroupFixture() (Config, Resource) {
	cfg := Config{
		Package:        "provider",
		ClientsImport:  "github.com/terraprovider/terraform-provider-exo/internal/clients",
		ClientField:    "EXO",
		BindingsImport: "github.com/terraprovider/go-exoscc/exo",
		BindingsPkg:    "exo",
	}
	r := Resource{
		Noun:        "RoleGroup",
		TFName:      "role_group",
		Description: "A management role group.",
		Attributes: []Attribute{
			{TFName: "name", Field: "Name", APIName: "Name", Type: TypeString, Required: true, Replace: true, Description: "Unique name.", InCreate: true},
			{TFName: "description", Field: "Description", APIName: "Description", Type: TypeString, Computed: true, Description: "Description.", InCreate: true, InUpdate: true},
			{TFName: "display_name", Field: "DisplayName", APIName: "DisplayName", Type: TypeString, Computed: true, Description: "Display name.", InCreate: true, InUpdate: true},
			{TFName: "roles", Field: "Roles", APIName: "Roles", Type: TypeStringSet, Computed: true, Replace: true, Description: "Roles.", InCreate: true},
		},
		Create: Op{Method: "NewRoleGroup", Params: "NewRoleGroupParams"},
		Read:   Op{Method: "GetRoleGroup", Params: "GetRoleGroupParams", IdentityField: "Identity"},
		Update: Op{Method: "SetRoleGroup", Params: "SetRoleGroupParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveRoleGroup", Params: "RemoveRoleGroupParams", IdentityField: "Identity"},
	}
	return cfg, r
}

func TestGenerateRoleGroupIsValidGo(t *testing.T) {
	cfg, r := roleGroupFixture()
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err) // format.Source failure surfaces the bad source
	}
	if len(files) != 3 { // resource + data source + registration
		t.Fatalf("want 3 files, got %d", len(files))
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
	if res == "" || ds == "" {
		t.Fatalf("missing generated files: resource=%v data_source=%v", res != "", ds != "")
	}
	for _, want := range []string{
		"func NewRoleGroupDataSource() datasource.DataSource",
		"readRoleGroup(ctx, obj, &data)",
		"d.client.EXO.GetRoleGroup(ctx",
	} {
		if !strings.Contains(ds, want) {
			t.Errorf("data source missing %q", want)
		}
	}

	for _, want := range []string{
		"func NewRoleGroupResource() resource.Resource",
		"type roleGroupModel struct",
		"exo.NewRoleGroupParams{",
		"r.client.EXO.NewRoleGroup(ctx, p)",
		"resourcex.LoadUntil(ctx, consistency.Config{}, get, reflected)",
		"reconcile.KeepStr(cfg.Description, read.Description)",
		"stringplanmodifier.RequiresReplace()", // name replace
		"setplanmodifier.RequiresReplace()",    // roles replace
		`resp.TypeName = req.ProviderTypeName + "_role_group"`,
	} {
		if !strings.Contains(res, want) {
			t.Errorf("generated source missing %q", want)
		}
	}
}

// TestPointerParamEmitsPointerAccessors covers the tri-state (Nullable<T>) path
// used by the Teams config surface, whose bindings take *bool / *string so an
// explicit false / "" is distinguishable from unset.
func TestPointerParamEmitsPointerAccessors(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "CS",
		BindingsImport: "github.com/terraprovider/go-teams/cs", BindingsPkg: "cs",
	}
	r := Resource{
		Noun: "TeamsMeetingPolicy", TFName: "teams_meeting_policy", Description: "A meeting policy.",
		Attributes: []Attribute{
			{TFName: "allow_meet_now", Field: "AllowMeetNow", APIName: "AllowMeetNow", Type: TypeBool, Computed: true, PointerParam: true, Description: "Allow meet now.", InCreate: true, InUpdate: true},
			{TFName: "description", Field: "Description", APIName: "Description", Type: TypeString, Computed: true, PointerParam: true, Description: "Description.", InCreate: true, InUpdate: true},
		},
		Create: Op{Method: "NewCsTeamsMeetingPolicy", Params: "NewCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Read:   Op{Method: "GetCsTeamsMeetingPolicy", Params: "GetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Update: Op{Method: "SetCsTeamsMeetingPolicy", Params: "SetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveCsTeamsMeetingPolicy", Params: "RemoveCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res string
	for _, f := range files {
		if f.Name == "teams_meeting_policy_resource.go" {
			res = string(f.Content)
		}
	}
	for _, want := range []string{
		"plan.AllowMeetNow.ValueBoolPointer()",
		"plan.Description.ValueStringPointer()",
	} {
		if !strings.Contains(res, want) {
			t.Errorf("generated source missing pointer accessor %q", want)
		}
	}
	if strings.Contains(res, "plan.AllowMeetNow.ValueBool()") {
		t.Error("PointerParam bool must use ValueBoolPointer(), not ValueBool()")
	}
}

// TestSparseWriteOmitsUnsetAndUnchanged covers the Teams tri-state write path:
// with SparseWrite, create must guard each field on IsUnknown/IsNull (so an
// unconfigured Optional+Computed *bool is not force-set to false) and update must
// guard on Equal(state) (so unchanged fields are not re-sent). Both would
// otherwise 403 on permission-gated Teams toggles.
func TestSparseWriteOmitsUnsetAndUnchanged(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "CS",
		BindingsImport: "github.com/terraprovider/go-teams/cs", BindingsPkg: "cs",
	}
	r := Resource{
		Noun: "MeetingPolicy", TFName: "meeting_policy", Description: "A meeting policy.",
		IdentityIsName: true, SparseWrite: true,
		Attributes: []Attribute{
			{TFName: "allow_meet_now", Field: "AllowMeetNow", APIName: "AllowMeetNow", Type: TypeBool, Computed: true, PointerParam: true, Description: "Allow meet now.", InCreate: true, InUpdate: true},
			{TFName: "auto_admitted_users", Field: "AutoAdmittedUsers", APIName: "AutoAdmittedUsers", Type: TypeString, Computed: true, Description: "Auto-admit.", InCreate: true, InUpdate: true},
		},
		Create: Op{Method: "NewCsTeamsMeetingPolicy", Params: "NewCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Read:   Op{Method: "GetCsTeamsMeetingPolicy", Params: "GetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Update: Op{Method: "SetCsTeamsMeetingPolicy", Params: "SetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveCsTeamsMeetingPolicy", Params: "RemoveCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res string
	for _, f := range files {
		if f.Name == "meeting_policy_resource.go" {
			res = string(f.Content)
		}
	}
	for _, want := range []string{
		// create: empty struct, guarded assignment (not a struct-literal field).
		// The guard is on config (what the operator wrote) so a ModifyPlan-filled
		// plan value is not mistaken for a configured one; the value is still taken
		// from plan (== config for set attributes).
		"cs.NewCsTeamsMeetingPolicyParams{}",
		"resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)",
		"if !config.AllowMeetNow.IsNull() {",
		"p.AllowMeetNow = plan.AllowMeetNow.ValueBoolPointer()",
		"if !config.AutoAdmittedUsers.IsNull() {",
		// update: only send changed
		"if !plan.AllowMeetNow.Equal(state.AllowMeetNow) {",
		"if !plan.AutoAdmittedUsers.Equal(state.AutoAdmittedUsers) {",
	} {
		if !strings.Contains(res, want) {
			t.Errorf("sparse-write source missing %q", want)
		}
	}
	// The unconditional struct-literal form must NOT appear for a sparse resource.
	if strings.Contains(res, "AllowMeetNow: plan.AllowMeetNow.ValueBoolPointer(),") {
		t.Error("SparseWrite create must guard fields, not emit them as struct-literal members")
	}
	// Newline-anchored: the guarded form is indented one tab deeper, so this only
	// matches the unconditional single-tab statement we must not emit.
	if strings.Contains(res, "\n\tsp.AllowMeetNow = plan.AllowMeetNow.ValueBoolPointer()\n") {
		t.Error("SparseWrite update must guard fields on Equal(state), not assign unconditionally")
	}
}

// TestAdoptIdentityGeneratesModifyPlanAndAdoptBranch verifies that an
// IdentityIsName resource with a reserved AdoptIdentity emits (a) a Create adopt
// short-circuit that applies Set instead of New, (b) a Delete that drops the
// reserved identity from state instead of removing it, and (c) ModifyPlan that
// reads the object during plan to fill unset attributes for a real delta.
func TestAdoptIdentityGeneratesModifyPlanAndAdoptBranch(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "CS",
		BindingsImport: "github.com/terraprovider/go-teams/cs", BindingsPkg: "cs",
	}
	r := Resource{
		Noun: "MeetingPolicy", TFName: "meeting_policy", Description: "A meeting policy.",
		IdentityIsName: true, SparseWrite: true, AdoptIdentity: "Global",
		Attributes: []Attribute{
			{TFName: "allow_meet_now", Field: "AllowMeetNow", APIName: "AllowMeetNow", Type: TypeBool, Computed: true, PointerParam: true, Description: "Allow meet now.", InCreate: true, InUpdate: true},
		},
		Create: Op{Method: "NewCsTeamsMeetingPolicy", Params: "NewCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Read:   Op{Method: "GetCsTeamsMeetingPolicy", Params: "GetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Update: Op{Method: "SetCsTeamsMeetingPolicy", Params: "SetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveCsTeamsMeetingPolicy", Params: "RemoveCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res string
	for _, f := range files {
		if f.Name == "meeting_policy_resource.go" {
			res = string(f.Content)
		}
	}
	for _, want := range []string{
		"resource.ResourceWithModifyPlan", // interface assertion (gofmt aligns the =)
		// Create adopt short-circuit: Set, not New, for the reserved identity.
		"if plan.Identity.ValueString() == \"Global\" {",
		"r.client.CS.SetCsTeamsMeetingPolicy(ctx, sp)",
		// Delete drops the reserved identity from state instead of Remove.
		"if r.identityOf(state) == \"Global\" {",
		"MeetingPolicy Global not deleted",
		// ModifyPlan reads on create and fills unknowns from the current object.
		"func (r *meetingPolicyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {",
		"if identity != \"Global\" {",
		"read" + r.Noun + "(ctx, obj, &cur)",
		"if plan.AllowMeetNow.IsUnknown() {",
		"plan.AllowMeetNow = cur.AllowMeetNow",
	} {
		if !strings.Contains(res, want) {
			t.Errorf("adopt/modifyplan source missing %q", want)
		}
	}
}

// TestIdentityIsName covers the Teams -Cs*Policy family where the -Identity is the
// user-chosen name and also the create key.
func TestIdentityIsName(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "CS",
		BindingsImport: "github.com/terraprovider/go-teams/cs", BindingsPkg: "cs",
	}
	r := Resource{
		Noun: "MeetingPolicy", TFName: "meeting_policy", Description: "A meeting policy.",
		IdentityIsName: true,
		Attributes: []Attribute{
			{TFName: "allow_meet_now", Field: "AllowMeetNow", APIName: "AllowMeetNow", Type: TypeBool, Computed: true, PointerParam: true, Description: "Allow meet now.", InCreate: true, InUpdate: true},
		},
		Create: Op{Method: "NewCsTeamsMeetingPolicy", Params: "NewCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Read:   Op{Method: "GetCsTeamsMeetingPolicy", Params: "GetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Update: Op{Method: "SetCsTeamsMeetingPolicy", Params: "SetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveCsTeamsMeetingPolicy", Params: "RemoveCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res string
	for _, f := range files {
		if f.Name == "meeting_policy_resource.go" {
			res = string(f.Content)
		}
	}
	// identity is a Required, RequiresReplace input (its description is unique to
	// the IdentityIsName branch, avoiding gofmt-alignment whitespace).
	if !strings.Contains(res, `Required: true, Description: "Name (Identity)`) {
		t.Error("identity attribute must be Required when IdentityIsName")
	}
	if !strings.Contains(res, "stringplanmodifier.RequiresReplace()") {
		t.Error("identity must be RequiresReplace when IdentityIsName")
	}
	// ...and it is passed to the create op.
	if !strings.Contains(res, "p.Identity = plan.Identity.ValueString()") {
		t.Error("Create must pass the identity/name to the New params when IdentityIsName")
	}
	// Create reads back by the user's identity (the New cmdlet may not echo the
	// created object), not by a field of the response.
	if !strings.Contains(res, "ident := plan.Identity.ValueString()") {
		t.Error("Create must refresh by plan.Identity when IdentityIsName (New may return no object)")
	}
	// The empty-response error must be a fallback *inside* the refresh miss, not a
	// hard pre-check that fires before read-back.
	if strings.Contains(res, "obj := firstObject(res.Value)\n\tif obj == nil {") {
		t.Error("obj==nil must be a refresh fallback, not a hard pre-check, for IdentityIsName")
	}
	// read<Noun> must NOT overwrite Identity from the read-back (it carries the
	// scoped "Tag:X" form while the operator configured the bare "X").
	readFn := res[strings.Index(res, "func readMeetingPolicy("):]
	if i := strings.Index(readFn, "\n}\n"); i >= 0 {
		readFn = readFn[:i]
	}
	if strings.Contains(readFn, "m.Identity = types.StringValue") {
		t.Error("readMeetingPolicy must not overwrite Identity for IdentityIsName")
	}
}

// TestAssignmentResource covers the per-user policy-assignment mode
// (Resource.Assignment): a two-field resource (user + policy_name) backed by the
// Grant-Cs<Type>Policy user grant, with an async poll-until-effective on
// create/update, an unassign (PolicyName="") on delete, and an effective-
// assignment read that drops the resource on drift.
func TestAssignmentResource(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "CS",
		BindingsImport: "github.com/terraprovider/go-teams/cs", BindingsPkg: "cs",
	}
	r := Resource{
		Noun: "TeamsMeetingPolicyAssignment", TFName: "teams_meeting_policy_assignment",
		Description:          "Assigns a TeamsMeetingPolicy instance to a user.",
		Assignment:           true,
		AssignmentPolicyType: "TeamsMeetingPolicy",
		AssignmentUserRead:   "EffectiveUserPolicy",
		Create:               Op{Method: "GrantCsTeamsMeetingPolicy", Params: "GrantCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err) // format.Source failure surfaces bad source
	}

	// No data source file (and no plural) is emitted for an assignment resource.
	var res, reg string
	for _, f := range files {
		switch f.Name {
		case "teams_meeting_policy_assignment_resource.go":
			res = string(f.Content)
		case "zz_generated_resources.go":
			reg = string(f.Content)
		case "teams_meeting_policy_assignment_data_source.go":
			t.Errorf("assignment resource must not emit a data source file")
		}
	}
	if res == "" {
		t.Fatal("missing generated assignment resource file")
	}
	if len(files) != 2 { // resource + registration only
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.Name
		}
		t.Fatalf("want 2 files (resource+registration), got %d: %v", len(files), names)
	}

	// Registration wires the resource constructor but NOT a data source constructor.
	if !strings.Contains(reg, "NewTeamsMeetingPolicyAssignmentResource,") {
		t.Error("registration missing the assignment resource constructor")
	}
	if strings.Contains(reg, "NewTeamsMeetingPolicyAssignmentDataSource") {
		t.Error("registration must not reference a data source for an assignment resource")
	}

	for _, want := range []string{
		"func NewTeamsMeetingPolicyAssignmentResource() resource.Resource",
		"type teamsMeetingPolicyAssignmentModel struct",
		`tfsdk:"user"`,        // gofmt pads the field/tag columns, so match the tag
		`tfsdk:"policy_name"`, // (not "<Field> types.String" with fixed spacing)
		`resp.TypeName = req.ProviderTypeName + "_teams_meeting_policy_assignment"`,
		// schema: user Required + RequiresReplace; policy_name Required (gofmt pads
		// the map-key column, so anchor on the value from `schema.` onward).
		`schema.StringAttribute{Required: true, Description: "User the policy is assigned to`,
		"stringplanmodifier.RequiresReplace()",
		`schema.StringAttribute{Required: true, Description: "Name of the TeamsMeetingPolicy instance to assign to the user."}`,
		// Create: grant with user in the identity field + PolicyName, then poll.
		"cs.GrantCsTeamsMeetingPolicyParams{Identity: user, PolicyName: name}",
		"r.client.CS.GrantCsTeamsMeetingPolicy(ctx,",
		"r.waitAssigned(ctx, user, name, &resp.Diagnostics)",
		// Read: effective assignment; drift removes the resource.
		`r.client.CS.EffectiveUserPolicy(ctx, user, "TeamsMeetingPolicy")`,
		"resp.State.RemoveResource(ctx)",
		// Delete: unassign via empty PolicyName.
		`cs.GrantCsTeamsMeetingPolicyParams{Identity: state.User.ValueString(), PolicyName: ""}`,
		// Async poll uses the shared consistency/resourcex helpers with a larger budget.
		"resourcex.LoadUntil(ctx, consistency.Config{Attempts: 20, Delay: 4 * time.Second}",
		"func(got string) bool { return got == want }",
		// Import keys id + user off the import id.
		`path.Root("user"), req.ID`,
	} {
		if !strings.Contains(res, want) {
			t.Errorf("assignment source missing %q", want)
		}
	}
}

// TestInt64Attribute covers the TypeInt path (int / *int64 bindings): int
// attributes become types.Int64 with a schema.Int64Attribute, the int64
// planmodifier import, pointer accessors for tri-state params, getInt read-back,
// and reconcile.KeepInt64.
func TestInt64Attribute(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "CS",
		BindingsImport: "github.com/terraprovider/go-teams/cs", BindingsPkg: "cs",
	}
	r := Resource{
		Noun: "OnlinePSTNGateway", TFName: "online_pstn_gateway", Description: "A gateway.",
		IdentityIsName: true, SparseWrite: true,
		Attributes: []Attribute{
			{TFName: "sip_signaling_port", Field: "SipSignalingPort", APIName: "SipSignalingPort", Type: TypeInt, Computed: true, PointerParam: true, Description: "SIP port.", InCreate: true, InUpdate: true},
		},
		Create: Op{Method: "NewCsOnlinePSTNGateway", Params: "NewCsOnlinePSTNGatewayParams", IdentityField: "Identity"},
		Read:   Op{Method: "GetCsOnlinePSTNGateway", Params: "GetCsOnlinePSTNGatewayParams", IdentityField: "Identity"},
		Update: Op{Method: "SetCsOnlinePSTNGateway", Params: "SetCsOnlinePSTNGatewayParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveCsOnlinePSTNGateway", Params: "RemoveCsOnlinePSTNGatewayParams", IdentityField: "Identity"},
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res string
	for _, f := range files {
		if f.Name == "online_pstn_gateway_resource.go" {
			res = string(f.Content)
		}
	}
	for _, want := range []string{
		"SipSignalingPort types.Int64", // model field (gofmt pads the tag column)
		"tfsdk:\"sip_signaling_port\"",
		"schema.Int64Attribute{",
		"resource/schema/int64planmodifier",
		"int64planmodifier.UseStateForUnknown()",
		"plan.SipSignalingPort.ValueInt64Pointer()",
		"types.Int64Value(getInt(obj, \"SipSignalingPort\"))",
		"reconcile.KeepInt64(",
	} {
		if !strings.Contains(res, want) {
			t.Errorf("int64 attribute missing %q", want)
		}
	}
}
