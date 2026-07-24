package genframework

import (
	"strings"
	"testing"
)

// TestDataSourceOnly verifies a Get-only noun emits a self-contained data source
// (its own model + read<Noun>) and no managed resource.
func TestDataSourceOnly(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "EXO",
		BindingsImport: "github.com/terraprovider/go-exoscc/exo", BindingsPkg: "exo",
	}
	r := Resource{
		Noun: "MailboxStatistics", TFName: "mailbox_statistics", Description: "Mailbox statistics.",
		Attributes: []Attribute{
			{TFName: "display_name", Field: "DisplayName", APIName: "DisplayName", Type: TypeString, Computed: true, Description: "Display name."},
		},
		Read:           Op{Method: "GetMailboxStatistics", Params: "GetMailboxStatisticsParams", IdentityField: "Identity"},
		DataSourceOnly: true,
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var names []string
	var ds, reg string
	for _, f := range files {
		names = append(names, f.Name)
		if f.Name == "mailbox_statistics_data_source.go" {
			ds = string(f.Content)
		}
		if f.Name == "zz_generated_resources.go" {
			reg = string(f.Content)
		}
	}
	if strings.Contains(strings.Join(names, ","), "mailbox_statistics_resource.go") {
		t.Errorf("DataSourceOnly must not emit a resource file; got %v", names)
	}
	for _, want := range []string{
		"type mailboxStatisticsModel struct",                  // self-contained model
		"func readMailboxStatistics(ctx context.Context, obj", // self-contained read
		"func NewMailboxStatisticsDataSource() datasource.DataSource",
	} {
		if !strings.Contains(ds, want) {
			t.Errorf("standalone data source missing %q", want)
		}
	}
	if strings.Contains(reg, "NewMailboxStatisticsResource") {
		t.Error("DataSourceOnly noun must not be registered as a resource")
	}
	if !strings.Contains(reg, "NewMailboxStatisticsDataSource") {
		t.Error("DataSourceOnly noun must be registered as a data source")
	}
}

// TestRawJSONDataSource verifies a raw-json data source exposes id/identity/name/
// display_name + a json attribute, with no per-property schema.
func TestRawJSONDataSource(t *testing.T) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "EXO",
		BindingsImport: "github.com/terraprovider/go-exoscc/exo", BindingsPkg: "exo",
	}
	r := Resource{
		Noun: "FederationTrust", TFName: "federation_trust", Description: "Federation trust.",
		Read:           Op{Method: "GetFederationTrust", Params: "GetFederationTrustParams", IdentityField: "Identity"},
		DataSourceOnly: true,
		RawJSON:        true,
	}
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var ds string
	for _, f := range files {
		if f.Name == "federation_trust_data_source.go" {
			ds = string(f.Content)
		}
	}
	for _, want := range []string{
		"`tfsdk:\"json\"`",
		"decode with jsondecode()",
		"m.JSON = types.StringValue(toJSON(obj))",
		"GetFederationTrust(ctx, exo.GetFederationTrustParams{Identity: identity})",
	} {
		if !strings.Contains(ds, want) {
			t.Errorf("raw-json data source missing %q", want)
		}
	}
}
