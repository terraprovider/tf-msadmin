package genframework

import (
	"strings"
	"testing"
)

// typedFixture models a go-exoscc typed-bindings resource: typed bool/int params
// bound as *bool / *int64 (PointerParam) and a multi-value list param.
func typedFixture() (Config, Resource) {
	cfg, r := roleGroupFixture()
	r.Noun, r.TFName = "HostedContentFilterPolicy", "hosted_content_filter_policy"
	r.Attributes = []Attribute{
		{TFName: "name", Field: "Name", APIName: "Name", Type: TypeString, Required: true, Replace: true, InCreate: true},
		{TFName: "enabled", Field: "Enabled", APIName: "Enabled", Type: TypeBool, Computed: true, PointerParam: true, InCreate: true, InUpdate: true},
		{TFName: "threshold", Field: "Threshold", APIName: "Threshold", Type: TypeInt, Computed: true, PointerParam: true, InCreate: true, InUpdate: true},
		{TFName: "allow_list", Field: "AllowList", APIName: "AllowList", Type: TypeStringSet, Computed: true, InCreate: true, InUpdate: true},
	}
	r.Create = Op{Method: "NewHostedContentFilterPolicy", Params: "NewHostedContentFilterPolicyParams"}
	r.Read = Op{Method: "GetHostedContentFilterPolicy", Params: "GetHostedContentFilterPolicyParams", IdentityField: "Identity"}
	r.Update = Op{Method: "SetHostedContentFilterPolicy", Params: "SetHostedContentFilterPolicyParams", IdentityField: "Identity"}
	r.Delete = Op{Method: "RemoveHostedContentFilterPolicy", Params: "RemoveHostedContentFilterPolicyParams", IdentityField: "Identity"}
	return cfg, r
}

func genOne(t *testing.T, cfg Config, r Resource) string {
	t.Helper()
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return string(files[0].Content)
}

func wantAll(t *testing.T, src string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("generated code missing %q", w)
		}
	}
}

func TestTypedWritesGuardUnknownAndClearLists(t *testing.T) {
	cfg, r := typedFixture()
	src := genOne(t, cfg, r)
	wantAll(t, src,
		// Create: unknown pointer values stay nil; an empty set is not sent to New.
		"if !plan.Enabled.IsUnknown() {\n\t\tp.Enabled = plan.Enabled.ValueBoolPointer()",
		"if !plan.Threshold.IsUnknown() {\n\t\tp.Threshold = plan.Threshold.ValueInt64Pointer()",
		"if v := toStringSlice(ctx, plan.AllowList, &resp.Diagnostics); len(v) > 0 {\n\t\tp.AllowList = v",
		// Update: pointer guard, and a known set is sent as a non-nil slice when
		// non-empty or changed (so clearing sends []).
		"if !plan.Enabled.IsUnknown() {\n\t\tsp.Enabled = plan.Enabled.ValueBoolPointer()",
		"len(v) > 0 || !plan.AllowList.Equal(state.AllowList) {\n\t\t\tsp.AllowList = append([]string{}, v...)",
	)
	// Guarded fields must not also appear unguarded in the create literal.
	if strings.Contains(src, "Enabled: plan.Enabled") || strings.Contains(src, "AllowList: toStringSlice") {
		t.Errorf("guarded field emitted in create literal:\n%s", src)
	}
}

func TestConfigCreateSendsKnownEmptySet(t *testing.T) {
	cfg, r := typedFixture()
	r.Config, r.Singleton = true, true
	src := genOne(t, cfg, r)
	wantAll(t, src,
		"if !plan.AllowList.IsNull() && !plan.AllowList.IsUnknown() {\n\t\tsp.AllowList = append([]string{}, toStringSlice(ctx, plan.AllowList, &resp.Diagnostics)...)",
	)
}

func TestSparseWriteTypedFields(t *testing.T) {
	cfg, r := typedFixture()
	r.SparseWrite = true
	src := genOne(t, cfg, r)
	wantAll(t, src,
		"if !config.Enabled.IsNull() {\n\t\tif !plan.Enabled.IsUnknown() {",
		"if !plan.AllowList.Equal(state.AllowList) {\n\t\tif !plan.AllowList.IsNull() && !plan.AllowList.IsUnknown() {\n\t\t\tsp.AllowList = append([]string{}, ",
	)
}

func TestComputedIdentityUsesStateForUnknown(t *testing.T) {
	cfg, r := typedFixture()
	src := genOne(t, cfg, r)
	if l := identityLine(src); !strings.Contains(l, "UseStateForUnknown()") {
		t.Errorf("computed identity lacks UseStateForUnknown: %s", l)
	}

	// An in-place-updatable Name may change the identity read back, so the prior
	// value must not be carried into the plan.
	r.Attributes[0].Replace, r.Attributes[0].Required = false, false
	r.Attributes[0].Computed, r.Attributes[0].InUpdate = true, true
	src = genOne(t, cfg, r)
	if l := identityLine(src); strings.Contains(l, "UseStateForUnknown()") {
		t.Errorf("identity of a renamable object must not use UseStateForUnknown: %s", l)
	}
}

// identityLine returns the generated schema line for the identity attribute.
func identityLine(src string) string {
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, `"identity":`) {
			return l
		}
	}
	return ""
}

func TestObjectRequiresString(t *testing.T) {
	cfg, r := typedFixture()
	r.Attributes[1].Object = true // a bool marked Object
	if _, err := Generate(cfg, []Resource{r}); err == nil || !strings.Contains(err.Error(), "Object requires TypeString") {
		t.Fatalf("want Object/TypeString error, got %v", err)
	}
}
