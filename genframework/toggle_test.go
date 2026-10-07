package genframework

import (
	"strings"
	"testing"
)

// ruleFixture models an Exchange rule: `enabled` is passed to New-* only, read
// from State, and toggled in place via Enable-/Disable-<Noun>.
func ruleFixture() (Config, Resource) {
	cfg, _ := roleGroupFixture()
	r := Resource{
		Noun: "SafeLinksRule", TFName: "safe_links_rule", Description: "A Safe Links rule.",
		Attributes: []Attribute{
			{TFName: "name", Field: "Name", APIName: "Name", Type: TypeString, Required: true, Replace: true, InCreate: true},
			{TFName: "priority", Field: "Priority", APIName: "Priority", Type: TypeInt, Computed: true, PointerParam: true, InCreate: true, InUpdate: true},
			{TFName: "enabled", Field: "Enabled", APIName: "Enabled", Type: TypeBool, Computed: true, PointerParam: true, InCreate: true,
				StateField: "State",
				Toggle: &Toggle{
					EnableMethod: "EnableSafeLinksRule", EnableParams: "EnableSafeLinksRuleParams",
					DisableMethod: "DisableSafeLinksRule", DisableParams: "DisableSafeLinksRuleParams",
					IdentityField: "Identity",
				}},
		},
		Create: Op{Method: "NewSafeLinksRule", Params: "NewSafeLinksRuleParams"},
		Read:   Op{Method: "GetSafeLinksRule", Params: "GetSafeLinksRuleParams", IdentityField: "Identity"},
		Update: Op{Method: "SetSafeLinksRule", Params: "SetSafeLinksRuleParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveSafeLinksRule", Params: "RemoveSafeLinksRuleParams", IdentityField: "Identity"},
	}
	return cfg, r
}

func TestStateFieldRead(t *testing.T) {
	cfg, r := ruleFixture()
	wantAll(t, genOne(t, cfg, r), `m.Enabled = types.BoolValue(getStateBool(obj, "State", "Enabled"))`)
}

func TestToggleUpdate(t *testing.T) {
	cfg, r := ruleFixture()
	src := genOne(t, cfg, r)
	wantAll(t, src,
		// Create still passes it to New-*.
		"p.Enabled = plan.Enabled.ValueBoolPointer()",
		// Update: toggle via Enable-/Disable- after the Set write, only on change,
		// targeting the same identity, with the not-found retry.
		"resourcex.RetryWriteCall(ctx, consistency.Config{}, r.client.EXO.SetSafeLinksRule, sp, isNotFound)",
		"if !plan.Enabled.IsUnknown() && !plan.Enabled.IsNull() && !plan.Enabled.Equal(state.Enabled) {\n\t\tif plan.Enabled.ValueBool() {",
		"resourcex.RetryWriteCall(ctx, consistency.Config{}, r.client.EXO.EnableSafeLinksRule, exo.EnableSafeLinksRuleParams{Identity: id}, isNotFound); err != nil {\n\t\t\t\tresp.Diagnostics.AddError(\"Enable-SafeLinksRule failed\", err.Error())",
		"resourcex.RetryWriteCall(ctx, consistency.Config{}, r.client.EXO.DisableSafeLinksRule, exo.DisableSafeLinksRuleParams{Identity: id}, isNotFound); err != nil {\n\t\t\t\tresp.Diagnostics.AddError(\"Disable-SafeLinksRule failed\", err.Error())",
	)
	// The post-write refresh waits until the toggle is visible: a changed toggle
	// extends the reflected predicate with the same State/APIName mapping.
	wantAll(t, src, "if !cfg.Enabled.IsUnknown() && !cfg.Enabled.IsNull() && !cfg.Enabled.Equal(state.Enabled) {\n\t\tprev, want := reflected, cfg.Enabled.ValueBool()\n\t\treflected = func(obj map[string]any) bool { return prev(obj) && getStateBool(obj, \"State\", \"Enabled\") == want }")
	// Never written into the Set params, and not replace-only.
	if strings.Contains(src, "sp.Enabled") {
		t.Error("Toggle attribute must not be written into the Set params")
	}
	if strings.Contains(src, "boolplanmodifier.RequiresReplace()") {
		t.Error("Toggle attribute must not force replacement")
	}
	// The toggle runs after the Set write and before the post-write refresh.
	set := strings.Index(src, "r.client.EXO.SetSafeLinksRule, sp")
	tog := strings.Index(src, "r.client.EXO.EnableSafeLinksRule")
	pred := strings.Index(src, "prev, want := reflected")
	ref := strings.Index(src, "r.refresh(ctx, id, &plan, &resp.Diagnostics, reflected)")
	if !(set >= 0 && set < tog && tog < pred && pred < ref) {
		t.Errorf("want Set < toggle < predicate < refresh, got %d, %d, %d, %d", set, tog, pred, ref)
	}
}

func TestToggleValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Resource)
		want   string
	}{
		{"non-bool StateField", func(r *Resource) { r.Attributes[1].StateField = "State" }, "StateField requires TypeBool"},
		{"non-bool Toggle", func(r *Resource) { r.Attributes[1].Toggle = r.Attributes[2].Toggle }, "Toggle requires TypeBool"},
		{"Toggle + Replace", func(r *Resource) { r.Attributes[2].Replace = true }, "Toggle cannot be combined with Replace"},
		{"Toggle + InUpdate", func(r *Resource) { r.Attributes[2].InUpdate = true }, "Toggle cannot be combined with InUpdate"},
		{"Toggle on Config", func(r *Resource) { r.Config = true }, "not supported on Config or AdoptIdentity"},
		{"Toggle on AdoptIdentity", func(r *Resource) { r.IdentityIsName, r.AdoptIdentity = true, "Global" }, "not supported on Config or AdoptIdentity"},
		{"Toggle missing method", func(r *Resource) {
			tg := *r.Attributes[2].Toggle
			tg.DisableMethod = ""
			r.Attributes[2].Toggle = &tg
		}, "needs Enable/Disable methods and params"},
	}
	for _, c := range cases {
		cfg, r := ruleFixture()
		c.mutate(&r)
		if _, err := Generate(cfg, []Resource{r}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.name, c.want, err)
		}
	}
}
