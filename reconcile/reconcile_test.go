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
