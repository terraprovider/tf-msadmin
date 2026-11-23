package genframework

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

const groupGolden = "internal/grouptest/provider/meeting_policy_group_assignment_resource.go"

func groupAssignmentFixture() (Config, Resource) {
	const fx = "github.com/terraprovider/tf-msadmin/genframework/internal/grouptest/"
	cfg := Config{Package: "provider", ClientsImport: fx + "clients", ClientField: "CS", TenantField: "TenantID", BindingsImport: fx + "cs", BindingsPkg: "cs"}
	r := Resource{
		Noun: "MeetingPolicyGroupAssignment", TFName: "meeting_policy_group_assignment",
		Description: "Assigns a TeamsMeetingPolicy instance to a group.",
		Assignment:  true, AssignmentGroup: true, AssignmentPolicyType: "TeamsMeetingPolicy",
		Create: Op{Method: "GrantCsTeamsMeetingPolicy", Params: "GrantCsTeamsMeetingPolicyParams"},
	}
	return cfg, r
}

// TestGroupAssignmentGolden pins the generated group assignment resource to the
// fixture package, which compiles it against a fake service and runs the
// lifecycle test (internal/grouptest/provider). Regenerate with -update.
func TestGroupAssignmentGolden(t *testing.T) {
	cfg, r := groupAssignmentFixture()
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) != 2 { // resource + registration, no data source
		t.Fatalf("want 2 files, got %d", len(files))
	}
	var res, reg string
	for _, f := range files {
		switch f.Name {
		case "meeting_policy_group_assignment_resource.go":
			res = string(f.Content)
		case "zz_generated_resources.go":
			reg = string(f.Content)
		}
	}
	if !strings.Contains(reg, "NewMeetingPolicyGroupAssignmentResource,") || strings.Contains(reg, "DataSource,") {
		t.Errorf("registration must list the resource and no data source:\n%s", reg)
	}
	for _, want := range []string{
		"cs.GrantCsTeamsMeetingPolicyParams{Group: group, PolicyName: want, Rank: &rank}",
		"cs.GrantCsTeamsMeetingPolicyParams{Group: group}",
		`r.client.CS.GroupPolicyAssignment(ctx, group, "TeamsMeetingPolicy")`,
		"meetingPolicyGroupAssignmentResourceMu.LoadOrStore(r.client.TenantID, &sync.Mutex{})",
	} {
		if !strings.Contains(res, want) {
			t.Errorf("generated source missing %q", want)
		}
	}
	// Without TenantField the lock is one per policy type across all tenants.
	cfg.TenantField = ""
	noTenant, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range noTenant {
		if f.Name == "meeting_policy_group_assignment_resource.go" && !strings.Contains(string(f.Content), `LoadOrStore("", &sync.Mutex{})`) {
			t.Error("without TenantField the lock key must be the empty string")
		}
	}
	if *update {
		if err := os.WriteFile(groupGolden, []byte(res), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(groupGolden)
	if err != nil {
		t.Fatal(err)
	}
	if string(golden) != res {
		t.Errorf("%s is stale; run go test ./genframework -run TestGroupAssignmentGolden -update", groupGolden)
	}
}

func TestGroupAssignmentValidate(t *testing.T) {
	_, r := groupAssignmentFixture()
	r.Assignment = false
	if _, err := Generate(Config{}, []Resource{r}); err == nil {
		t.Error("AssignmentGroup without Assignment must be rejected")
	}
}
