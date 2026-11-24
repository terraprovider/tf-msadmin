package genframework

import (
	"sort"
	"strings"
)

// registrationHelper is a function emitted into zz_generated_resources.go when
// a generated resource or data source uses it. Emitting these keeps them out of
// the helper contract each provider hand-writes (see Config).
type registrationHelper struct {
	name    string
	imports []string
	source  string
	used    func(r Resource, a Attribute) bool
}

const (
	importAttr               = "github.com/hashicorp/terraform-plugin-framework/attr"
	importPath               = "github.com/hashicorp/terraform-plugin-framework/path"
	importTypes              = "github.com/hashicorp/terraform-plugin-framework/types"
	importContext            = "context"
	importStrings            = "strings"
	importPlanmodifier       = "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	importBoolplanmodifier   = "github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	importStringplanmodifier = "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

var registrationHelpers = []registrationHelper{
	{
		name:    "replaceIfWriteOnlyRecorded",
		imports: []string{importContext, importPlanmodifier, importStringplanmodifier},
		used:    func(r Resource, a Attribute) bool { return !r.DataSourceOnly && a.replaceIfRecorded() },
		source: `// replaceIfWriteOnlyRecorded forces replacement for a changed write-only
// create-time value only when state records one. Nothing reads the value
// back, so an empty state value (e.g. after import) says nothing about what
// the object was created with; setting it then only records it.
func replaceIfWriteOnlyRecorded(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = req.StateValue.ValueString() != ""
}
`,
	},
	{
		name:    "failOnChangeBool",
		imports: []string{importContext, importPlanmodifier, importBoolplanmodifier},
		used:    func(r Resource, a Attribute) bool { return !r.DataSourceOnly && a.ChangeError != "" },
		source: `// failOnChangeBool fails the plan with msg when a configured value differs
// from the value in state, instead of replacing the object. A value left
// unconfigured (unknown here) and a new object never fail.
func failOnChangeBool(msg string) boolplanmodifier.RequiresReplaceIfFunc {
	return func(_ context.Context, req planmodifier.BoolRequest, resp *boolplanmodifier.RequiresReplaceIfFuncResponse) {
		if req.PlanValue.IsUnknown() || req.StateValue.IsNull() || req.StateValue.IsUnknown() {
			return
		}
		resp.Diagnostics.AddAttributeError(req.Path, "Change would replace the object", msg)
	}
}
`,
	},
	{
		name:    "unknownIfChangedBool",
		imports: []string{importContext, importAttr, importPath, importPlanmodifier, importTypes},
		used:    func(r Resource, a Attribute) bool { return !r.DataSourceOnly && len(a.ChangesWith) > 0 },
		source: `// unknownIfChangedBool plans an unconfigured bool as unknown when one of the
// named attributes changes, because that change also changes its read-back
// value (see genframework Attribute.ChangesWith).
func unknownIfChangedBool(attrs ...string) planmodifier.Bool { return unknownIfChangedBoolModifier(attrs) }

type unknownIfChangedBoolModifier []string

func (m unknownIfChangedBoolModifier) Description(context.Context) string {
	return "Planned as unknown when a linked attribute changes and this one is not configured."
}

func (m unknownIfChangedBoolModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m unknownIfChangedBoolModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.ConfigValue.IsNull() || req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	for _, name := range m {
		var planned, prior attr.Value
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(name), &planned)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(name), &prior)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if planned != nil && !planned.IsUnknown() && !planned.Equal(prior) {
			resp.PlanValue = types.BoolUnknown()
			return
		}
	}
}
`,
	},
	{
		name:    "stateBoolIs",
		imports: []string{importStrings},
		used:    func(_ Resource, a Attribute) bool { return a.customStateValue() },
		source: `// stateBoolIs is getStateBool with a match value other than "Enabled": true
// when the string property stateKey equals value (case-insensitive), else
// the bool property boolKey when stateKey is absent.
func stateBoolIs(m map[string]any, stateKey, value, boolKey string) bool {
	if s, ok := m[stateKey].(string); ok {
		return strings.EqualFold(s, value)
	}
	return getBool(m, boolKey)
}
`,
	},
	{
		name:    "stateString",
		imports: []string{importStrings},
		used:    func(_ Resource, a Attribute) bool { return len(a.StateMap) > 0 },
		source: `// stateString maps the string property key through values (keys compared
// case-insensitively). An absent or unmapped value reads as "".
func stateString(m map[string]any, key string, values map[string]string) string {
	s, _ := m[key].(string)
	for k, v := range values {
		if strings.EqualFold(k, s) {
			return v
		}
	}
	return ""
}
`,
	},
	{
		name:    "stateStringReflects",
		imports: []string{importStrings},
		used:    func(r Resource, a Attribute) bool { return !r.DataSourceOnly && a.InUpdate && len(a.StateMap) > 0 },
		source: `// stateStringReflects reports whether the mapped string property key equals
// want (case-insensitive). An object without the property counts as
// reflected, so a post-write wait cannot stall on it.
func stateStringReflects(m map[string]any, key string, values map[string]string, want string) bool {
	if m[key] == nil {
		return true
	}
	return strings.EqualFold(stateString(m, key, values), want)
}
`,
	},
}

// usedHelpers returns the registration helpers resources need and their imports, sorted.
func usedHelpers(resources []Resource) ([]registrationHelper, []string) {
	var used []registrationHelper
	imports := map[string]bool{}
	for _, h := range registrationHelpers {
		need := false
		for _, r := range resources {
			for _, a := range r.Attributes {
				need = need || h.used(r, a)
			}
		}
		if need {
			used = append(used, h)
			for _, i := range h.imports {
				imports[i] = true
			}
		}
	}
	var list []string
	for i := range imports {
		list = append(list, i)
	}
	sort.Strings(list)
	return used, list
}

// registrationImports renders the import block: the standard library, then the framework packages.
func registrationImports(extra []string) string {
	var std, ext []string
	for _, i := range append([]string{
		"github.com/hashicorp/terraform-plugin-framework/datasource",
		"github.com/hashicorp/terraform-plugin-framework/resource",
	}, extra...) {
		if strings.Contains(i, ".") {
			ext = append(ext, i)
		} else {
			std = append(std, i)
		}
	}
	sort.Strings(std)
	sort.Strings(ext)
	var b strings.Builder
	b.WriteString("import (\n")
	for _, i := range std {
		b.WriteString("\t\"" + i + "\"\n")
	}
	if len(std) > 0 {
		b.WriteString("\n")
	}
	for _, i := range ext {
		b.WriteString("\t\"" + i + "\"\n")
	}
	b.WriteString(")\n\n")
	return b.String()
}
