package genframework

import (
	"strings"
	"testing"
)

func configResource(singleton bool) (Config, Resource) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "EXO",
		BindingsImport: "github.com/terraprovider/go-exoscc/exo", BindingsPkg: "exo",
	}
	upIdent := "Identity"
	rdIdent := "Identity"
	if singleton {
		upIdent, rdIdent = "", ""
	}
	r := Resource{
		Noun: "AuditConfig", TFName: "audit_config", Description: "Audit config.",
		Attributes: []Attribute{
			{TFName: "enabled", Field: "Enabled", APIName: "Enabled", Type: TypeBool, Computed: true, Description: "Enabled.", InCreate: true, InUpdate: true},
		},
		Read:      Op{Method: "GetAuditConfig", Params: "GetAuditConfigParams", IdentityField: rdIdent},
		Update:    Op{Method: "SetAuditConfig", Params: "SetAuditConfigParams", IdentityField: upIdent},
		Config:    true,
		Singleton: singleton,
	}
	return cfg, r
}

func TestConfigSingletonGeneration(t *testing.T) {
	cfg, r := configResource(true)
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res string
	for _, f := range files {
		if f.Name == "audit_config_resource.go" {
			res = string(f.Content)
		}
	}
	for _, want := range []string{
		"r.client.EXO.SetAuditConfig(ctx, sp)",          // Create adopts via Set
		"resp.Diagnostics.AddWarning",                   // no-op delete warns
		"exo.GetAuditConfigParams{})",                   // keyless read (singleton)
		`schema.StringAttribute{Computed: true, Descri`, // identity is computed for a singleton
	} {
		if !strings.Contains(res, want) {
			t.Errorf("singleton config missing %q", want)
		}
	}
	if strings.Contains(res, "EXO.NewAuditConfig(ctx") {
		t.Error("config resource must not call the New cmdlet")
	}
}

func TestConfigPerObjectRequiresIdentity(t *testing.T) {
	cfg, r := configResource(false)
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var res string
	for _, f := range files {
		if f.Name == "audit_config_resource.go" {
			res = string(f.Content)
		}
	}
	if !strings.Contains(res, `"identity": schema.StringAttribute{Required: true`) {
		t.Error("per-object config must make identity a required input")
	}
	if !strings.Contains(res, "sp.Identity = plan.Identity.ValueString()") {
		t.Error("per-object config Set must target the configured identity")
	}
}
