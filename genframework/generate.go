package genframework

import (
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
		ds, err := genDataSource(cfg, r)
		if err != nil {
			return nil, fmt.Errorf("%s data source: %w", r.Noun, err)
		}
		files = append(files, File{Name: r.TFName + "_data_source.go", Content: ds})
	}

	reg, err := genRegistration(cfg, sorted)
	if err != nil {
		return nil, err
	}
	files = append(files, File{Name: "zz_generated_resources.go", Content: reg})
	return files, nil
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
func (r Resource) hasSet() bool     { return r.hasType(TypeStringSet) || r.Members != nil }

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
	default:
		if a.PointerParam {
			return "plan." + a.Field + ".ValueStringPointer()"
		}
		return "plan." + a.Field + ".ValueString()"
	}
}

// readAssign renders the readInto assignment.
func (a Attribute) readAssign() string {
	switch a.Type {
	case TypeBool:
		return fmt.Sprintf("m.%s = types.BoolValue(getBool(obj, %q))", a.Field, a.APIName)
	case TypeStringSet:
		return fmt.Sprintf("m.%s = stringSetValue(ctx, getStringSlice(obj, %q))", a.Field, a.APIName)
	default:
		return fmt.Sprintf("m.%s = types.StringValue(getString(obj, %q))", a.Field, a.APIName)
	}
}
