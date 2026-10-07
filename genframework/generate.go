package genframework

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"
)

// File is one generated output file.
type File struct {
	Name    string // suggested file name, e.g. "role_group_resource.go"
	Content []byte // gofmt-formatted source
}

// Generate produces one resource file per resource plus a registration file
// (zz_generated_resources.go) that lists their constructors. All output is
// gofmt-formatted; a formatting error surfaces the offending source for
// debugging.
func Generate(cfg Config, resources []Resource) ([]File, error) {
	for _, r := range resources {
		if err := r.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", r.Noun, err)
		}
	}
	var files []File
	sorted := append([]Resource(nil), resources...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Noun < sorted[j].Noun })

	for _, r := range sorted {
		if !r.DataSourceOnly {
			src, err := genResource(cfg, r)
			if err != nil {
				return nil, fmt.Errorf("%s resource: %w", r.Noun, err)
			}
			files = append(files, File{Name: r.TFName + "_resource.go", Content: src})
		}
		if r.Assignment {
			// A per-user assignment has no object identity to read back — no data
			// source is emitted (and none is registered, see genRegistration).
			continue
		}
		ds, err := genDataSource(cfg, r)
		if err != nil {
			return nil, fmt.Errorf("%s data source: %w", r.Noun, err)
		}
		files = append(files, File{Name: r.TFName + "_data_source.go", Content: ds})

		if r.Plural {
			pds, err := genPluralDataSource(cfg, r)
			if err != nil {
				return nil, fmt.Errorf("%s plural data source: %w", r.Noun, err)
			}
			files = append(files, File{Name: r.pluralTFName() + "_data_source.go", Content: pds})
		}
	}

	reg, err := genRegistration(cfg, sorted)
	if err != nil {
		return nil, err
	}
	files = append(files, File{Name: "zz_generated_resources.go", Content: reg})
	return files, nil
}

// validate rejects attribute combinations the emitter cannot render correctly.
func (r Resource) validate() error {
	for _, a := range r.Attributes {
		// Object is the System.Object JSON round-trip (a string attribute written
		// into an `any` field). A param whose declared type is bool/int/list must be
		// modelled as that type with Object=false, so it takes the typed write path.
		if a.Object && a.Type != TypeString {
			return fmt.Errorf("attribute %s: Object requires TypeString", a.TFName)
		}
		if a.Delta && a.Type != TypeStringSet {
			return fmt.Errorf("attribute %s: Delta requires TypeStringSet", a.TFName)
		}
	}
	return nil
}

// hasUpdateDelta reports whether a Set-* write may clear a list via a Remove
// delta, which needs the current() re-read emitted by genCurrent.
func (r Resource) hasUpdateDelta() bool {
	for _, a := range r.Attributes {
		if a.Delta && a.InUpdate {
			return true
		}
	}
	return false
}

// genCurrent emits, before a Set-* params build, a lazily evaluated current()
// that re-reads the object (at most once) so a cleared Delta list can send
// Remove = the server's current values. It returns nil when the object cannot
// be read. Nothing is emitted when no attribute uses a delta.
func genCurrent(b *bytes.Buffer, r Resource, idExpr string) {
	if !r.hasUpdateDelta() {
		return
	}
	model := r.model()
	fmt.Fprintf(b, "\tvar cur *%s\n", model)
	fmt.Fprintf(b, "\tcurrent := func() *%s {\n", model)
	fmt.Fprintf(b, "\t\tif cur == nil {\n\t\t\tvar m %s\n", model)
	fmt.Fprintf(b, "\t\t\tif !r.refresh(ctx, %s, &m, &resp.Diagnostics, nil) {\n\t\t\t\treturn nil\n\t\t\t}\n", idExpr)
	fmt.Fprintf(b, "\t\t\tcur = &m\n\t\t}\n\t\treturn cur\n\t}\n")
}

// identityStable reports whether the computed identity cannot change across an
// in-place update, so it may keep its prior value via UseStateForUnknown. It can
// change only when it may be read back from an updatable Name.
func (r Resource) identityStable() bool {
	n := r.field("Name")
	return n == nil || n.Replace || !n.InUpdate
}

func gofmt(src string) ([]byte, error) {
	out, err := format.Source([]byte(src))
	if err != nil {
		return nil, fmt.Errorf("format: %w\n----\n%s", err, src)
	}
	return out, nil
}

// ---- naming helpers ----

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func (r Resource) recv() string     { return lowerFirst(r.Noun) + "Resource" }
func (r Resource) model() string    { return lowerFirst(r.Noun) + "Model" }
func (r Resource) ctor() string     { return "New" + r.Noun + "Resource" }
func (r Resource) hasMembers() bool { return r.Members != nil }

// adoptsExisting reports whether the resource, on create, manages an object that
// already exists — a config, or a CRUD resource with a reserved AdoptIdentity like
// "Global". Such resources emit ModifyPlan (read the object during plan to show a
// real delta instead of "(known after apply)") and gate their sparse write on
// config rather than plan, so ModifyPlan-filled values are not sent. Only
// meaningful for SparseWrite providers (Teams); others keep full re-send semantics.
func (r Resource) adoptsExisting() bool {
	return r.SparseWrite && (r.Config || (r.IdentityIsName && r.AdoptIdentity != ""))
}
func (r Resource) hasSet() bool { return r.hasType(TypeStringSet) || r.Members != nil }

// plural* name the emitted "list" data source, which shares the singular's
// element model and read<Noun> mapper.
func (r Resource) pluralRecv() string   { return lowerFirst(r.Noun) + "ListDataSource" }
func (r Resource) pluralModel() string  { return lowerFirst(r.Noun) + "ListModel" }
func (r Resource) pluralCtor() string   { return "New" + r.Noun + "ListDataSource" }
func (r Resource) pluralTFName() string { return pluralizeSnake(r.TFName) }

// pluralizeSnake pluralises the last word of a snake_case name using simple
// English rules: "compliance_case" -> "compliance_cases", "retention_policy" ->
// "retention_policies", "dlp_edm_schema" -> "dlp_edm_schemas".
func pluralizeSnake(s string) string {
	i := strings.LastIndex(s, "_")
	head, word := "", s
	if i >= 0 {
		head, word = s[:i+1], s[i+1:]
	}
	switch {
	case word == "":
		return s
	case strings.HasSuffix(word, "y") && len(word) >= 2 && !isVowel(word[len(word)-2]):
		word = word[:len(word)-1] + "ies"
	case strings.HasSuffix(word, "s"), strings.HasSuffix(word, "x"), strings.HasSuffix(word, "z"),
		strings.HasSuffix(word, "ch"), strings.HasSuffix(word, "sh"):
		word += "es"
	default:
		word += "s"
	}
	return head + word
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// memberReadExpr renders firstNonEmptyStr(getString(mm,"K1"), getString(mm,"K2"), ...)
// over the member collection's read-back keys, used to extract a member identity.
func (mc MemberCollection) memberReadExpr() string {
	keys := mc.ReadKeys
	if len(keys) == 0 {
		keys = []string{"PrimarySmtpAddress", "Name", "Identity"}
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("getString(mm, %q)", k)
	}
	return "firstNonEmptyStr(" + strings.Join(parts, ", ") + ")"
}

// hasBoolMod reports whether any Bool attribute emits a plan modifier (only
// RequiresReplace does), which requires the boolplanmodifier import.
func (r Resource) hasBoolMod() bool {
	for _, a := range r.Attributes {
		if a.Type == TypeBool && a.planModifiers() != "" {
			return true
		}
	}
	return false
}

// hasInt64Mod reports whether any Int64 attribute emits a plan modifier, which
// requires the int64planmodifier import.
func (r Resource) hasInt64Mod() bool {
	for _, a := range r.Attributes {
		if a.Type == TypeInt && a.planModifiers() != "" {
			return true
		}
	}
	return false
}

// hasSetMod reports whether any set attribute (or the members set) emits a plan
// modifier, which requires the setplanmodifier import. A Required-only set has none.
func (r Resource) hasSetMod() bool {
	if r.Members != nil {
		return true
	}
	for _, a := range r.Attributes {
		if a.Type == TypeStringSet && a.planModifiers() != "" {
			return true
		}
	}
	return false
}

func (r Resource) hasName() bool { return r.field("Name") != nil }
func (r Resource) cmdlet(v string) string {
	return v + "-" + r.Noun
}

// identityReadExpr renders the expression that extracts the stored identity
// value from a read-back object, trying the generic keys plus the resource's
// specific IdentityReadField (e.g. "PlaceMailboxId").
func (r Resource) identityReadExpr() string {
	parts := []string{`getString(obj, "Identity")`, `getString(obj, "Guid")`}
	if f := r.IdentityReadField; f != "" && f != "Identity" && f != "Guid" {
		parts = append(parts, fmt.Sprintf("getString(obj, %q)", f))
	}
	parts = append(parts, `getString(obj, "Name")`)
	return "firstNonEmptyStr(" + strings.Join(parts, ", ") + ")"
}

func (r Resource) hasType(t AttrType) bool {
	for _, a := range r.Attributes {
		if a.Type == t {
			return true
		}
	}
	return false
}

func (r Resource) field(name string) *Attribute {
	for i := range r.Attributes {
		if r.Attributes[i].Field == name {
			return &r.Attributes[i]
		}
	}
	return nil
}

// ---- per-attribute rendering ----

func (a Attribute) tfType() string {
	switch a.Type {
	case TypeBool:
		return "Bool"
	case TypeStringSet:
		return "Set"
	case TypeInt:
		return "Int64"
	default:
		return "String"
	}
}

func (a Attribute) keepFn() string {
	switch a.Type {
	case TypeBool:
		return "KeepBool"
	case TypeStringSet:
		return "KeepSet"
	case TypeInt:
		return "KeepInt64"
	default:
		return "KeepStr"
	}
}

// modelField renders the struct field for the model.
func (a Attribute) modelField() string {
	return fmt.Sprintf("%s types.%s `tfsdk:%q`", a.Field, a.tfType(), a.TFName)
}

// schemaAttr renders the schema.Attribute literal.
func (a Attribute) schemaAttr() string {
	var b strings.Builder
	mods := a.planModifiers()
	switch a.Type {
	case TypeBool:
		b.WriteString("schema.BoolAttribute{")
		b.WriteString(a.modeFields())
		if a.Sensitive {
			b.WriteString("Sensitive: true, ")
		}
		fmt.Fprintf(&b, "Description: %q, ", a.Description)
		if mods != "" {
			fmt.Fprintf(&b, "PlanModifiers: []planmodifier.Bool{%s}, ", mods)
		}
		b.WriteString("}")
	case TypeStringSet:
		b.WriteString("schema.SetAttribute{ElementType: types.StringType, ")
		b.WriteString(a.modeFields())
		fmt.Fprintf(&b, "Description: %q, ", a.Description)
		if mods != "" {
			fmt.Fprintf(&b, "PlanModifiers: []planmodifier.Set{%s}, ", mods)
		}
		b.WriteString("}")
	case TypeInt:
		b.WriteString("schema.Int64Attribute{")
		b.WriteString(a.modeFields())
		if a.Sensitive {
			b.WriteString("Sensitive: true, ")
		}
		fmt.Fprintf(&b, "Description: %q, ", a.Description)
		if mods != "" {
			fmt.Fprintf(&b, "PlanModifiers: []planmodifier.Int64{%s}, ", mods)
		}
		b.WriteString("}")
	default:
		b.WriteString("schema.StringAttribute{")
		b.WriteString(a.modeFields())
		if a.Sensitive {
			b.WriteString("Sensitive: true, ")
		}
		fmt.Fprintf(&b, "Description: %q, ", a.Description)
		if mods != "" {
			fmt.Fprintf(&b, "PlanModifiers: []planmodifier.String{%s}, ", mods)
		}
		b.WriteString("}")
	}
	return b.String()
}

func (a Attribute) modeFields() string {
	if a.Required {
		return "Required: true, "
	}
	return "Optional: true, Computed: true, "
}

func (a Attribute) planModifiers() string {
	var mods []string
	pkg := strings.ToLower(a.tfType()) + "planmodifier" // string/bool/set planmodifier
	if a.Replace {
		mods = append(mods, pkg+".RequiresReplace()")
	}
	// Computed attributes keep their prior value when the plan leaves them
	// unknown, so an unrelated update does not churn them (and, for
	// RequiresReplace ones, does not spuriously force replacement).
	if a.Computed {
		mods = append(mods, pkg+".UseStateForUnknown()")
	}
	return strings.Join(mods, ", ")
}

// createValue renders the params assignment value for create/update. When the
// binding field is a pointer (PointerParam — tri-state APIs), it emits the
// *-pointer accessor so an explicit false / "" is sent and an unset (null/unknown)
// plan value marshals to nil (omitted) rather than a zero value.
func (a Attribute) planValue() string {
	switch a.Type {
	case TypeBool:
		if a.PointerParam {
			return "plan." + a.Field + ".ValueBoolPointer()"
		}
		return "plan." + a.Field + ".ValueBool()"
	case TypeStringSet:
		return "toStringSlice(ctx, plan." + a.Field + ", &resp.Diagnostics)"
	case TypeInt:
		if a.PointerParam {
			return "plan." + a.Field + ".ValueInt64Pointer()"
		}
		return "plan." + a.Field + ".ValueInt64()"
	default:
		if a.PointerParam {
			return "plan." + a.Field + ".ValueStringPointer()"
		}
		return "plan." + a.Field + ".ValueString()"
	}
}

// writeSite says which kind of write a params assignment is part of. It only
// matters for sets, where an empty list is meaningful (it clears the property).
type writeSite int

const (
	// siteNew: a New-<Noun> create. There is nothing to clear yet, so a set is
	// sent only when it has elements.
	siteNew writeSite = iota
	// siteSet: a Set-<Noun> applying configured values (adopt/config create, or a
	// sparse update already gated on "changed"). A known set is always sent, an
	// empty one as [] so it clears the existing list.
	siteSet
	// siteUpdate: a full re-send update. A known set is sent when it has elements
	// or differs from state, so clearing a list sends [] but an already-empty list
	// is not re-sent on every apply.
	siteUpdate
)

// guardedWrite reports whether the attribute's params assignment needs a
// per-value guard instead of an unconditional assignment: pointer fields (an
// unknown value would otherwise send &false / &0 / &"") and sets (nil means
// "not sent", a non-nil empty slice sends []).
func (a Attribute) guardedWrite() bool {
	return a.PointerParam || a.Type == TypeStringSet
}

// writeStmt renders the assignment of the attribute's plan value into params
// variable pv (e.g. "p", "sp"), terminated by a newline. Callers add their own
// outer gate (SparseWrite's config/changed checks); this adds only the
// type-specific guard so an unknown value is never sent:
//   - pointer fields are left nil while the plan value is unknown;
//   - sets are left nil while null/unknown, and otherwise get a non-nil slice
//     (possibly empty) according to site.
//
// The update site references `state`, which must be in scope.
func (a Attribute) writeStmt(pv string, site writeSite) string {
	f := a.Field
	switch {
	case a.Type == TypeStringSet:
		known := fmt.Sprintf("!plan.%s.IsNull() && !plan.%s.IsUnknown()", f, f)
		read := fmt.Sprintf("toStringSlice(ctx, plan.%s, &resp.Diagnostics)", f)
		switch {
		case site == siteNew:
			return fmt.Sprintf("if v := %s; len(v) > 0 {\n%s.%s = v\n}\n", read, pv, f)
		case a.Delta:
			// Set-* with a delta companion: non-empty is a full replace; empty
			// removes the current server values (an empty list would be ignored).
			// A full-resend update only clears when the set actually changed.
			remove := fmt.Sprintf("if c := current(); c != nil {\nif rm := toStringSlice(ctx, c.%s, &resp.Diagnostics); len(rm) > 0 {\n%s.%sDelta = listRemoveDelta(rm)\n}\n}\n", f, pv, f)
			if site == siteUpdate {
				remove = fmt.Sprintf("if !plan.%s.Equal(state.%s) {\n%s}\n", f, f, remove)
			}
			return fmt.Sprintf("if %s {\nif v := %s; len(v) > 0 {\n%s.%s = v\n} else {\n%s}\n}\n", known, read, pv, f, remove)
		case site == siteUpdate:
			return fmt.Sprintf("if %s {\nif v := %s; len(v) > 0 || !plan.%s.Equal(state.%s) {\n%s.%s = append([]string{}, v...)\n}\n}\n",
				known, read, f, f, pv, f)
		default:
			return fmt.Sprintf("if %s {\n%s.%s = append([]string{}, %s...)\n}\n", known, pv, f, read)
		}
	case a.PointerParam:
		return fmt.Sprintf("if !plan.%s.IsUnknown() {\n%s.%s = %s\n}\n", f, pv, f, a.planValue())
	default:
		return fmt.Sprintf("%s.%s = %s\n", pv, f, a.planValue())
	}
}

// readAssign renders the readInto assignment.
func (a Attribute) readAssign() string {
	if a.Object {
		// System.Object read-back is a structured value (array/object); serialize it
		// to a JSON string so it round-trips against a jsonencode() config.
		return fmt.Sprintf("m.%s = types.StringValue(getObjectJSON(obj, %q))", a.Field, a.APIName)
	}
	switch a.Type {
	case TypeBool:
		return fmt.Sprintf("m.%s = types.BoolValue(getBool(obj, %q))", a.Field, a.APIName)
	case TypeStringSet:
		return fmt.Sprintf("m.%s = stringSetValue(ctx, getStringSlice(obj, %q))", a.Field, a.APIName)
	case TypeInt:
		return fmt.Sprintf("m.%s = types.Int64Value(getInt(obj, %q))", a.Field, a.APIName)
	default:
		return fmt.Sprintf("m.%s = types.StringValue(getString(obj, %q))", a.Field, a.APIName)
	}
}
