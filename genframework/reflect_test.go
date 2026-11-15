package genframework

import (
	"strings"
	"testing"
)

// Update's post-write refresh waits until changed bool and int attributes read
// back with their new value, to ride out replication lag
// (terraform-provider-exo#27). A field the read-back doesn't carry is skipped.
func TestUpdateReflectsChangedScalars(t *testing.T) {
	cfg, r := ruleFixture()
	r.Attributes = append(r.Attributes,
		Attribute{TFName: "allow_smtp", Field: "AllowSmtp", APIName: "AllowSmtp", Type: TypeBool, Computed: true, PointerParam: true, InCreate: true, InUpdate: true},
		Attribute{TFName: "legacy_flag", Field: "LegacyFlag", APIName: "LegacyFlag", Type: TypeBool, Computed: true, InCreate: true, InUpdate: true},
		Attribute{TFName: "settings", Field: "Settings", APIName: "Settings", Type: TypeString, Object: true, Computed: true, InCreate: true, InUpdate: true},
	)
	src := genOne(t, cfg, r)
	wantAll(t, src,
		"if !cfg.AllowSmtp.IsUnknown() && !cfg.AllowSmtp.IsNull() && !cfg.AllowSmtp.Equal(state.AllowSmtp) {\n\t\tprev, want := reflected, cfg.AllowSmtp.ValueBool()\n\t\treflected = func(obj map[string]any) bool {\n\t\t\treturn prev(obj) && (obj[\"AllowSmtp\"] == nil || getBool(obj, \"AllowSmtp\") == want)\n\t\t}",
		"if !cfg.Priority.IsUnknown() && !cfg.Priority.IsNull() && !cfg.Priority.Equal(state.Priority) {\n\t\tprev, want := reflected, cfg.Priority.ValueInt64()\n\t\treflected = func(obj map[string]any) bool {\n\t\t\treturn prev(obj) && (obj[\"Priority\"] == nil || getInt(obj, \"Priority\") == want)\n\t\t}",
	)
	// A non-pointer bool never sends false, so waiting for it could stall; the
	// toggle has its own State-based predicate; Object attributes are normalized
	// by the API.
	for _, field := range []string{"LegacyFlag", "Settings"} {
		if strings.Contains(src, "reflected, cfg."+field+".") {
			t.Errorf("%s must not extend the reflected predicate", field)
		}
	}
	// The toggle (Enabled, StateField + Toggle) is covered once, by genToggleReflected.
	if n := strings.Count(src, "reflected, cfg.Enabled."); n != 1 {
		t.Errorf("toggle predicate emitted %d times, want 1 (genToggleReflected only)", n)
	}
	// The predicate is built after the write and before the refresh.
	set := strings.Index(src, "r.client.EXO.SetSafeLinksRule, sp")
	pred := strings.Index(src, "reflected, cfg.AllowSmtp.")
	ref := strings.Index(src, "r.refresh(ctx, id, &plan, &resp.Diagnostics, reflected)")
	if set < 0 || set >= pred || pred >= ref {
		t.Errorf("want Set < predicate < refresh, got %d, %d, %d", set, pred, ref)
	}
}

// A StateField bool that Set updates directly (no Toggle) is waited for with
// the same getStateBool mapping as the read, and only counts as absent when
// neither read-back property is present.
func TestUpdateReflectsStateFieldBool(t *testing.T) {
	cfg, r := ruleFixture()
	r.Attributes = append(r.Attributes,
		Attribute{TFName: "active", Field: "Active", APIName: "Active", Type: TypeBool, Computed: true, PointerParam: true, InCreate: true, InUpdate: true, StateField: "Mode"},
	)
	src := genOne(t, cfg, r)
	wantAll(t, src,
		`m.Active = types.BoolValue(getStateBool(obj, "Mode", "Active"))`,
		"if !cfg.Active.IsUnknown() && !cfg.Active.IsNull() && !cfg.Active.Equal(state.Active) {\n\t\tprev, want := reflected, cfg.Active.ValueBool()\n\t\treflected = func(obj map[string]any) bool {\n\t\t\treturn prev(obj) && ((obj[\"Mode\"] == nil && obj[\"Active\"] == nil) || getStateBool(obj, \"Mode\", \"Active\") == want)\n\t\t}",
	)
}
