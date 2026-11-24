package genframework

import (
	"strings"
	"testing"
)

// meetingPolicyFixture is a Teams policy: an IdentityIsName resource that
// adopts Global, so Create has both the adopt Set and the New path.
func meetingPolicyFixture() (Config, Resource) {
	cfg := Config{
		Package: "provider", ClientsImport: "example.com/clients", ClientField: "CS",
		BindingsImport: "github.com/terraprovider/go-teams/cs", BindingsPkg: "cs",
	}
	r := Resource{
		Noun: "MeetingPolicy", TFName: "meeting_policy", Description: "A meeting policy.",
		IdentityIsName: true, SparseWrite: true, AdoptIdentity: "Global",
		Attributes: []Attribute{
			{TFName: "allow_meet_now", Field: "AllowMeetNow", APIName: "AllowMeetNow", Type: TypeBool, Computed: true, PointerParam: true, InCreate: true, InUpdate: true},
		},
		Create: Op{Method: "NewCsTeamsMeetingPolicy", Params: "NewCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Read:   Op{Method: "GetCsTeamsMeetingPolicy", Params: "GetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Update: Op{Method: "SetCsTeamsMeetingPolicy", Params: "SetCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
		Delete: Op{Method: "RemoveCsTeamsMeetingPolicy", Params: "RemoveCsTeamsMeetingPolicyParams", IdentityField: "Identity"},
	}
	return cfg, r
}

func TestWriteConflictRetriesEveryMainWrite(t *testing.T) {
	cfg, r := meetingPolicyFixture()
	cfg.WriteConflict = "isWriteConflict"
	src := genOne(t, cfg, r)
	wantAll(t, src,
		// Create: New, and the adopt Set for Global.
		"res, err := resourcex.RetryConflictCall(ctx, consistency.Config{}, r.client.CS.NewCsTeamsMeetingPolicy, p, isNotFound, isWriteConflict)",
		"if _, err := resourcex.RetryConflictCall(ctx, consistency.Config{}, r.client.CS.SetCsTeamsMeetingPolicy, sp, isNotFound, isWriteConflict); err != nil {\n\t\t\tresp.Diagnostics.AddError(",
		// Update.
		"if _, err := resourcex.RetryConflictCall(ctx, consistency.Config{}, r.client.CS.SetCsTeamsMeetingPolicy, sp, isNotFound, isWriteConflict); err != nil {\n\t\tresp.Diagnostics.AddError(",
		// Delete: conflict retry only; not-found still means already gone.
		"if err := resourcex.RetryConflict(ctx, resourcex.ConflictConfig{}, func(ctx context.Context) error {\n\t\t_, err := r.client.CS.RemoveCsTeamsMeetingPolicy(ctx, cs.RemoveCsTeamsMeetingPolicyParams{Identity: r.identityOf(state)})\n\t\treturn err\n\t}, isWriteConflict); err != nil {\n\t\tif !isNotFound(err) {",
	)
	if strings.Contains(src, "resourcex.RetryWriteCall(") {
		t.Error("a main write still uses RetryWriteCall without the conflict retry")
	}
}

func TestWriteConflictRetriesToggles(t *testing.T) {
	cfg, r := ruleFixture()
	cfg.WriteConflict = "isWriteConflict"
	wantAll(t, genOne(t, cfg, r),
		"resourcex.RetryConflictCall(ctx, consistency.Config{}, r.client.EXO.EnableSafeLinksRule, exo.EnableSafeLinksRuleParams{Identity: id}, isNotFound, isWriteConflict)",
		"resourcex.RetryConflictCall(ctx, consistency.Config{}, r.client.EXO.DisableSafeLinksRule, exo.DisableSafeLinksRuleParams{Identity: id}, isNotFound, isWriteConflict)",
	)
}

func TestWriteConflictRetriesConfigSet(t *testing.T) {
	cfg, r := configResource(false)
	cfg.WriteConflict = "isWriteConflict"
	wantAll(t, genOne(t, cfg, r), "resourcex.RetryConflictCall(ctx, consistency.Config{}, r.client.EXO.SetAuditConfig, sp, isNotFound, isWriteConflict)")
}

// Without WriteConflict (exo, scc) the output is what it was: no conflict
// retry, and no new helper in the contract.
func TestWriteConflictUnsetKeepsOutput(t *testing.T) {
	for name, fixture := range map[string]func() (Config, Resource){
		"policy": meetingPolicyFixture, "rule": ruleFixture, "typed": typedFixture,
	} {
		cfg, r := fixture()
		src := genOne(t, cfg, r)
		if strings.Contains(src, "RetryConflict") || strings.Contains(src, "isWriteConflict") {
			t.Errorf("%s: conflict retry emitted without WriteConflict", name)
		}
		if !strings.Contains(src, "resourcex.RetryWriteCall(ctx, consistency.Config{}, r.client.") {
			t.Errorf("%s: main write lost RetryWriteCall", name)
		}
	}
}

func TestWriteConflictMustBeIdentifier(t *testing.T) {
	cfg, r := meetingPolicyFixture()
	cfg.WriteConflict = "is conflict"
	if _, err := Generate(cfg, []Resource{r}); err == nil {
		t.Fatal("Generate accepted a WriteConflict that is not a Go identifier")
	}
}
