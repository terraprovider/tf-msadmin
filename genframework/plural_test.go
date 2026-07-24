package genframework

import (
	"strings"
	"testing"
)

// TestPluralizeSnake checks the English pluralisation of the last word.
func TestPluralizeSnake(t *testing.T) {
	cases := map[string]string{
		"compliance_case":  "compliance_cases",
		"retention_policy": "retention_policies", // consonant + y -> ies
		"gateway":          "gateways",           // vowel + y -> s
		"dlp_edm_schema":   "dlp_edm_schemas",
		"mailbox":          "mailboxes", // x -> es
		"address":          "addresses", // s -> es
		"role_group":       "role_groups",
	}
	for in, want := range cases {
		if got := pluralizeSnake(in); got != want {
			t.Errorf("pluralizeSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPluralDataSource verifies that Plural emits a companion list data source
// that reuses the singular element model + read<Noun> and is registered.
func TestPluralDataSource(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "EXO",
		BindingsImport: "github.com/terraprovider/go-exoscc/exo", BindingsPkg: "exo",
	}
	r := Resource{
		Noun: "RetentionPolicy", TFName: "retention_policy", Description: "Retention policy.",
		Attributes: []Attribute{
			{TFName: "comment", Field: "Comment", APIName: "Comment", Type: TypeString, Computed: true, Description: "Comment."},
		},
		Read:           Op{Method: "GetRetentionPolicy", Params: "GetRetentionPolicyParams", IdentityField: "Identity"},
		DataSourceOnly: true,
		Plural:         true,
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var plural, reg string
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
		switch f.Name {
		case "retention_policies_data_source.go":
			plural = string(f.Content)
		case "zz_generated_resources.go":
			reg = string(f.Content)
		}
	}
	if plural == "" {
		t.Fatalf("no plural file emitted; got %v", names)
	}
	for _, want := range []string{
		"func NewRetentionPolicyListDataSource() datasource.DataSource",
		"type retentionPolicyListModel struct",
		"Items []retentionPolicyModel `tfsdk:\"retention_policies\"`",
		"schema.ListNestedAttribute{Computed: true",
		"exo.GetRetentionPolicyParams{}",
		"readRetentionPolicy(ctx, obj, &e)",
		"e.Identity = types.StringValue(", // list elements get their identity from the read-back

		`req.ProviderTypeName + "_retention_policies"`,
	} {
		if !strings.Contains(plural, want) {
			t.Errorf("plural data source missing %q", want)
		}
	}
	if !strings.Contains(reg, "NewRetentionPolicyListDataSource") {
		t.Error("plural data source not registered")
	}
}
