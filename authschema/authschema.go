// Package authschema provides the shared Terraform provider authentication
// surface for the Microsoft admin providers (Exchange Online, Purview, Teams).
//
// Every provider exposes the same block of attributes — aligned field-for-field
// with the hashicorp/azuread and azurerm providers, including the ARM_*/AZURE_*
// environment fallbacks and every OIDC/workload-identity flavour — so operators
// configure them identically. A provider embeds Model in its config struct,
// merges Attributes() into its schema, and calls Model.Config() to obtain the
// resolved authx.Config.
package authschema

import (
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/terraprovider/go-msadmin/authx"
)

// Model is the embedded auth portion of a provider's configuration model. A
// provider embeds it anonymously and adds its own provider-specific attributes:
//
//	type providerModel struct {
//	    authschema.Model
//	    Organization types.String `tfsdk:"organization"`
//	}
type Model struct {
	TenantID                  types.String `tfsdk:"tenant_id"`
	ClientID                  types.String `tfsdk:"client_id"`
	Environment               types.String `tfsdk:"environment"`
	ClientSecret              types.String `tfsdk:"client_secret"`
	ClientCertificate         types.String `tfsdk:"client_certificate"`
	ClientCertificatePath     types.String `tfsdk:"client_certificate_path"`
	ClientCertificatePassword types.String `tfsdk:"client_certificate_password"`
	UseOIDC                   types.Bool   `tfsdk:"use_oidc"`
	OIDCToken                 types.String `tfsdk:"oidc_token"`
	OIDCTokenFilePath         types.String `tfsdk:"oidc_token_file_path"`
	OIDCRequestToken          types.String `tfsdk:"oidc_request_token"`
	OIDCRequestURL            types.String `tfsdk:"oidc_request_url"`
	ADOServiceConnectionID    types.String `tfsdk:"ado_pipeline_service_connection_id"`
	UseCLI                    types.Bool   `tfsdk:"use_cli"`
	UseMSI                    types.Bool   `tfsdk:"use_msi"`
}

// Config overlays the explicitly-configured values onto the ARM_*/AZURE_*
// environment and returns the resolved authx.Config.
func (m Model) Config() authx.Config {
	return authx.FromEnv().Merge(authx.Config{
		Environment:               m.Environment.ValueString(),
		TenantID:                  m.TenantID.ValueString(),
		ClientID:                  m.ClientID.ValueString(),
		ClientSecret:              m.ClientSecret.ValueString(),
		ClientCertificate:         m.ClientCertificate.ValueString(),
		ClientCertificatePath:     m.ClientCertificatePath.ValueString(),
		ClientCertificatePassword: m.ClientCertificatePassword.ValueString(),
		UseOIDC:                   m.UseOIDC.ValueBool(),
		OIDCToken:                 m.OIDCToken.ValueString(),
		OIDCTokenFilePath:         m.OIDCTokenFilePath.ValueString(),
		OIDCRequestToken:          m.OIDCRequestToken.ValueString(),
		OIDCRequestURL:            m.OIDCRequestURL.ValueString(),
		ADOServiceConnectionID:    m.ADOServiceConnectionID.ValueString(),
		UseCLI:                    m.UseCLI.ValueBool(),
		UseMSI:                    m.UseMSI.ValueBool(),
	})
}

// Attributes returns the shared authentication attributes for a provider schema.
// Merge them into the provider's own attribute map.
func Attributes() map[string]schema.Attribute {
	s := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Description: desc}
	}
	sensitive := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Sensitive: true, Description: desc}
	}
	b := func(desc string) schema.BoolAttribute { return schema.BoolAttribute{Optional: true, Description: desc} }
	return map[string]schema.Attribute{
		"tenant_id":                          s("Tenant ID or primary domain. Env: ARM_TENANT_ID."),
		"client_id":                          s("Entra application (client) ID. Env: ARM_CLIENT_ID."),
		"environment":                        s("Cloud environment: public, usgovernment, dod, china. Env: ARM_ENVIRONMENT."),
		"client_secret":                      sensitive("Client secret. Env: ARM_CLIENT_SECRET."),
		"client_certificate":                 sensitive("Base64 PEM/PKCS#12 certificate bundle. Env: ARM_CLIENT_CERTIFICATE."),
		"client_certificate_path":            s("Path to a PEM/PKCS#12 certificate bundle. Env: ARM_CLIENT_CERTIFICATE_PATH."),
		"client_certificate_password":        sensitive("Certificate password. Env: ARM_CLIENT_CERTIFICATE_PASSWORD."),
		"use_oidc":                           b("Use OIDC/workload-identity federation. Env: ARM_USE_OIDC."),
		"oidc_token":                         sensitive("Pre-fetched OIDC assertion. Env: ARM_OIDC_TOKEN."),
		"oidc_token_file_path":               s("File containing the OIDC assertion. Env: ARM_OIDC_TOKEN_FILE_PATH / AZURE_FEDERATED_TOKEN_FILE."),
		"oidc_request_token":                 sensitive("OIDC request bearer (GitHub/ADO). Env: ARM_OIDC_REQUEST_TOKEN / ACTIONS_ID_TOKEN_REQUEST_TOKEN / SYSTEM_ACCESSTOKEN."),
		"oidc_request_url":                   s("OIDC request URL (GitHub/ADO). Env: ARM_OIDC_REQUEST_URL / ACTIONS_ID_TOKEN_REQUEST_URL / SYSTEM_OIDCREQUESTURI."),
		"ado_pipeline_service_connection_id": s("Azure DevOps workload-identity service connection ID. Env: ARM_OIDC_AZURE_SERVICE_CONNECTION_ID."),
		"use_cli":                            b("Authenticate via the Azure CLI. Env: ARM_USE_CLI."),
		"use_msi":                            b("Authenticate via a managed identity. Env: ARM_USE_MSI."),
	}
}
