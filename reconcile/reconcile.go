// Package reconcile holds the state-reconciliation rule shared by every
// Microsoft admin provider resource.
//
// The admin APIs are eventually consistent and sometimes normalise written
// values, so a read-back immediately after a create/update can disagree with
// the value the operator configured. Terraform rejects that as "inconsistent
// result after apply" for known planned values. The rule is therefore: for any
// attribute that was explicitly configured (known in the plan), the configured
// value is authoritative and must not be clobbered by the read; only
// computed/unset attributes take their freshly-read value.
package reconcile

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// KeepStr returns the configured value when it is known (explicitly set in the
// plan), otherwise the freshly-read value.
func KeepStr(cfg, read types.String) types.String {
	if !cfg.IsNull() && !cfg.IsUnknown() {
		return cfg
	}
	return read
}

// KeepSet is KeepStr for set attributes.
func KeepSet(cfg, read types.Set) types.Set {
	if !cfg.IsNull() && !cfg.IsUnknown() {
		return cfg
	}
	return read
}

// KeepBool is KeepStr for bool attributes.
func KeepBool(cfg, read types.Bool) types.Bool {
	if !cfg.IsNull() && !cfg.IsUnknown() {
		return cfg
	}
	return read
}

// KeepInt64 is KeepStr for int64 attributes.
func KeepInt64(cfg, read types.Int64) types.Int64 {
	if !cfg.IsNull() && !cfg.IsUnknown() {
		return cfg
	}
	return read
}

// KeepList is KeepStr for list attributes.
func KeepList(cfg, read types.List) types.List {
	if !cfg.IsNull() && !cfg.IsUnknown() {
		return cfg
	}
	return read
}

// KeepStrFold returns the prior (state) value when the freshly-read value
// differs from it only in letter case, otherwise the freshly-read value. It is
// applied on refresh (Read), where no configuration is available: the cmdlets
// accept enum-like strings case-insensitively but echo their own casing
// ("MODERATE" written, "Moderate" read), so storing the read-back verbatim makes
// every plan show a case-only update. A real change still comes through.
//
// ponytail: applies to every plain string, so a case-only edit of free text
// (a description) made outside Terraform is not detected. Restrict it to
// enum-valued attributes once the spec knows which strings are enums.
func KeepStrFold(prior, read types.String) types.String {
	if prior.IsNull() || prior.IsUnknown() || read.IsNull() || read.IsUnknown() {
		return read
	}
	if strings.EqualFold(prior.ValueString(), read.ValueString()) {
		return prior
	}
	return read
}

// ReflectsFields returns a predicate that reports whether a read-back object
// reflects the given configured string values (keyed by API field name). Only
// known (explicitly configured) values are checked; null/unknown are ignored.
// Used to wait out write-propagation lag after an update. Values are compared
// case-insensitively: a service that echoes its own casing of an enum value
// ("Moderate" for a written "MODERATE") has applied the write, and an exact
// comparison would spend the whole consistency budget waiting for it.
func ReflectsFields(want map[string]types.String, get func(obj map[string]any, field string) string) func(map[string]any) bool {
	return func(obj map[string]any) bool {
		for field, v := range want {
			if v.IsNull() || v.IsUnknown() {
				continue
			}
			if !strings.EqualFold(get(obj, field), v.ValueString()) {
				return false
			}
		}
		return true
	}
}
