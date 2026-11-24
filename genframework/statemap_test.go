package genframework

import (
	"strings"
	"testing"
)

// mailboxTypeFixture models New-Mailbox type switches read from
// RecipientTypeDetails, and Set-Mailbox -Type read through a value map.
func mailboxTypeFixture() (Config, Resource) {
	cfg, r := typedFixture()
	r.Attributes = append(r.Attributes,
		Attribute{TFName: "shared", Field: "Shared", APIName: "Shared", Type: TypeBool, Computed: true, Replace: true, InCreate: true,
			StateField: "RecipientTypeDetails", StateValue: "SharedMailbox", ChangeError: "use type to convert", ChangesWith: []string{"type"}},
		Attribute{TFName: "type", Field: "Type", APIName: "Type", Type: TypeString, Computed: true, Object: true, InCreate: false, InUpdate: true,
			StateField: "RecipientTypeDetails", StateMap: map[string]string{"UserMailbox": "Regular", "SharedMailbox": "Shared"}},
	)
	return cfg, r
}

func genFiles(t *testing.T, cfg Config, r Resource) (src, reg string) {
	t.Helper()
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return string(files[0].Content), string(files[len(files)-1].Content)
}

func TestStateValueReadsCustomMatch(t *testing.T) {
	cfg, r := mailboxTypeFixture()
	src, reg := genFiles(t, cfg, r)
	wantAll(t, src, `m.Shared = types.BoolValue(stateBoolIs(obj, "RecipientTypeDetails", "SharedMailbox", "Shared"))`)
	wantAll(t, reg, "func stateBoolIs(m map[string]any, stateKey, value, boolKey string) bool {", "\t\"strings\"\n")
}

func TestStateValueEnabledKeepsGetStateBool(t *testing.T) {
	cfg, r := typedFixture()
	r.Attributes = append(r.Attributes, Attribute{TFName: "enabled2", Field: "Enabled2", APIName: "Enabled2", Type: TypeBool, Computed: true, InCreate: true, StateField: "State", StateValue: "enabled"})
	src, reg := genFiles(t, cfg, r)
	wantAll(t, src, `m.Enabled2 = types.BoolValue(getStateBool(obj, "State", "Enabled2"))`)
	if strings.Contains(reg, "stateBoolIs") {
		t.Error("an Enabled match value emitted stateBoolIs")
	}
}

func TestStateMapReadAndUpdateWait(t *testing.T) {
	cfg, r := mailboxTypeFixture()
	src, reg := genFiles(t, cfg, r)
	args := `obj, "RecipientTypeDetails", map[string]string{"SharedMailbox": "Shared", "UserMailbox": "Regular"}`
	wantAll(t, src,
		"m.Type = types.StringValue(stateString("+args+"))",
		"return prev(obj) && stateStringReflects("+args+", want)",
	)
	if strings.Contains(src, `getObjectJSON(obj, "Type")`) {
		t.Error("a StateMap attribute is still read from APIName")
	}
	wantAll(t, reg,
		"func stateString(m map[string]any, key string, values map[string]string) string {",
		"func stateStringReflects(m map[string]any, key string, values map[string]string, want string) bool {",
	)
}

func TestChangeErrorFailsThePlan(t *testing.T) {
	cfg, r := mailboxTypeFixture()
	src, reg := genFiles(t, cfg, r)
	wantAll(t, src,
		`boolplanmodifier.RequiresReplaceIf(failOnChangeBool("use type to convert"), `,
		"\"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier\"",
	)
	if strings.Contains(src, `"shared":`) && strings.Contains(src, "boolplanmodifier.RequiresReplace(), boolplanmodifier.UseStateForUnknown()}") {
		t.Error("shared still has a plain RequiresReplace")
	}
	wantAll(t, reg,
		"func failOnChangeBool(msg string) boolplanmodifier.RequiresReplaceIfFunc {",
		"if req.PlanValue.IsUnknown() || req.StateValue.IsNull() || req.StateValue.IsUnknown() {",
		"resp.Diagnostics.AddAttributeError(req.Path, \"Change would replace the object\", msg)",
	)
}

func TestRegistrationImportsOnlyWhatHelpersUse(t *testing.T) {
	cfg, r := typedFixture()
	_, reg := genFiles(t, cfg, r)
	for _, h := range []string{"stateBoolIs", "stateString", "failOnChangeBool", `"strings"`, `"context"`} {
		if strings.Contains(reg, h) {
			t.Errorf("registration without state or guard attributes contains %s", h)
		}
	}
}

func TestStateAttributeValidation(t *testing.T) {
	cases := map[string]Attribute{
		"StateField on a string without StateMap": {TFName: "x", Field: "X", Type: TypeString, StateField: "S"},
		"StateValue without StateField":           {TFName: "x", Field: "X", Type: TypeBool, StateValue: "V"},
		"StateMap on a bool":                      {TFName: "x", Field: "X", Type: TypeBool, StateField: "S", StateMap: map[string]string{"a": "b"}},
		"ChangeError without Replace":             {TFName: "x", Field: "X", Type: TypeBool, ChangeError: "no"},
		"ChangeError on a write-only bool":        {TFName: "x", Field: "X", Type: TypeBool, Computed: true, Replace: true, WriteOnly: true, ChangeError: "no"},
		"ChangeError on a string":                 {TFName: "x", Field: "X", Type: TypeString, Replace: true, ChangeError: "no"},
		"ChangesWith on a non-computed bool":      {TFName: "x", Field: "X", Type: TypeBool, Required: true, ChangesWith: []string{"name"}},
		"StateMap keys equal ignoring case":       {TFName: "x", Field: "X", Type: TypeString, StateField: "S", StateMap: map[string]string{"Shared": "a", "shared": "b"}},
		"StateMap with an empty key":              {TFName: "x", Field: "X", Type: TypeString, StateField: "S", StateMap: map[string]string{"": "a"}},
		"ChangesWith naming no attribute":         {TFName: "x", Field: "X", Type: TypeBool, Computed: true, ChangesWith: []string{"typo"}},
		"ChangesWith naming itself":               {TFName: "x", Field: "X", Type: TypeBool, Computed: true, ChangesWith: []string{"x"}},
	}
	for name, a := range cases {
		cfg, r := typedFixture()
		r.Attributes = append(r.Attributes, a)
		if _, err := Generate(cfg, []Resource{r}); err == nil {
			t.Errorf("%s: Generate accepted it", name)
		}
	}
}

func TestChangesWithPlansUnknown(t *testing.T) {
	cfg, r := mailboxTypeFixture()
	src, reg := genFiles(t, cfg, r)
	wantAll(t, src, `boolplanmodifier.UseStateForUnknown(), unknownIfChangedBool("type")}`)
	wantAll(t, reg,
		"func unknownIfChangedBool(attrs ...string) planmodifier.Bool {",
		// An unconfigured linked attribute keeps its state value; a configured
		// unknown one (another resource's output) may change.
		"if configured == nil || configured.IsNull() {\n\t\t\tcontinue\n\t\t}",
		"if configured.IsUnknown() || planned == nil || !planned.Equal(prior) {\n\t\t\tresp.PlanValue = types.BoolUnknown()",
		"\t\"github.com/hashicorp/terraform-plugin-framework/attr\"\n",
	)
}

// A non-Object StateMap string is read from StateField, so the Update wait must
// not also compare its raw APIName property, which the read never uses.
func TestStateMapNotWaitedOnByAPIName(t *testing.T) {
	cfg, r := mailboxTypeFixture()
	for i := range r.Attributes {
		if r.Attributes[i].TFName == "type" {
			r.Attributes[i].Object = false
		}
	}
	src, _ := genFiles(t, cfg, r)
	wantAll(t, src, "return prev(obj) && stateStringReflects(")
	if strings.Contains(src, `"Type": cfg.Type`) {
		t.Error("the Update wait compares a StateMap attribute's APIName")
	}
}
