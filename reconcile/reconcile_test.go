package reconcile

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func strSet(ss ...string) types.Set {
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	return types.SetValueMust(types.StringType, vals)
}

func TestKeepStr(t *testing.T) {
	read := types.StringValue("read")
	if got := KeepStr(types.StringValue("cfg"), read); got.ValueString() != "cfg" {
		t.Errorf("known cfg should win: %s", got)
	}
	if got := KeepStr(types.StringNull(), read); got.ValueString() != "read" {
		t.Errorf("null cfg should take read: %s", got)
	}
	if got := KeepStr(types.StringUnknown(), read); got.ValueString() != "read" {
		t.Errorf("unknown cfg should take read: %s", got)
	}
}

func TestKeepSet(t *testing.T) {
	cfg := strSet("a")
	read := strSet("b")
	if got := KeepSet(cfg, read); !got.Equal(cfg) {
		t.Errorf("known cfg set should win")
	}
	if got := KeepSet(types.SetNull(types.StringType), read); !got.Equal(read) {
		t.Errorf("null cfg should take read")
	}
}

func TestReflectsFields(t *testing.T) {
	get := func(obj map[string]any, field string) string {
		if v, ok := obj[field].(string); ok {
			return v
		}
		return ""
	}
	pred := ReflectsFields(map[string]types.String{
		"Description": types.StringValue("new"),
		"DisplayName": types.StringNull(), // ignored
	}, get)

	if pred(map[string]any{"Description": "old"}) {
		t.Error("should not reflect when Description mismatches")
	}
	if !pred(map[string]any{"Description": "new", "DisplayName": "whatever"}) {
		t.Error("should reflect when known fields match (null ignored)")
	}
}

func TestKeepStrFold(t *testing.T) {
	s := types.StringValue
	cases := []struct {
		name              string
		prior, read, want types.String
	}{
		{"case-only difference keeps prior", s("MODERATE"), s("Moderate"), s("MODERATE")},
		{"real change takes read", s("MODERATE"), s("Strict"), s("Strict")},
		{"equal value", s("Moderate"), s("Moderate"), s("Moderate")},
		{"null prior (import) takes read", types.StringNull(), s("Moderate"), s("Moderate")},
		{"unknown prior takes read", types.StringUnknown(), s("Moderate"), s("Moderate")},
		{"cleared by service takes read", s("MODERATE"), types.StringNull(), types.StringNull()},
		{"empty read is a real change", s("MODERATE"), s(""), s("")},
	}
	for _, c := range cases {
		if got := KeepStrFold(c.prior, c.read); !got.Equal(c.want) {
			t.Errorf("%s: KeepStrFold(%v, %v) = %v, want %v", c.name, c.prior, c.read, got, c.want)
		}
	}
}

func TestReflectsFieldsIgnoresCase(t *testing.T) {
	get := func(obj map[string]any, field string) string {
		v, _ := obj[field].(string)
		return v
	}
	pred := ReflectsFields(map[string]types.String{"GiphyRatingType": types.StringValue("MODERATE")}, get)
	if !pred(map[string]any{"GiphyRatingType": "Moderate"}) {
		t.Error("service casing of the written value should count as reflected")
	}
	if pred(map[string]any{"GiphyRatingType": "Strict"}) {
		t.Error("a different value must not count as reflected")
	}
}
