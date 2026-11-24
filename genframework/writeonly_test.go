package genframework

import (
	"strings"
	"testing"
)

// writeOnlyFixture has a create-time secret (Replace) and an updatable one, both
// write-only and Optional+Computed, like New-Mailbox -Password and
// Set-Mailbox -RoomMailboxPassword.
func writeOnlyFixture() (Config, Resource) {
	cfg, r := typedFixture()
	r.Attributes = append(r.Attributes,
		Attribute{TFName: "password", Field: "Password", APIName: "Password", Type: TypeString, Computed: true, Sensitive: true, WriteOnly: true, Replace: true, InCreate: true},
		Attribute{TFName: "room_password", Field: "RoomPassword", APIName: "RoomPassword", Type: TypeString, Computed: true, Sensitive: true, WriteOnly: true, InCreate: true, InUpdate: true},
	)
	return cfg, r
}

func TestWriteOnlyIsNotReadBack(t *testing.T) {
	cfg, r := writeOnlyFixture()
	src := genOne(t, cfg, r)
	for _, f := range []string{"Password", "RoomPassword"} {
		if strings.Contains(src, "m."+f+" = ") {
			t.Errorf("read assigns write-only %s", f)
		}
	}
}

func TestWriteOnlyUnconfiguredIsNullAfterApply(t *testing.T) {
	cfg, r := writeOnlyFixture()
	src := genOne(t, cfg, r)
	wantAll(t, src,
		"read.Password = reconcile.KeepStr(cfg.Password, read.Password)\n\tif read.Password.IsUnknown() {\n\t\tread.Password = types.StringNull()\n\t}",
		"if read.RoomPassword.IsUnknown() {\n\t\tread.RoomPassword = types.StringNull()\n\t}",
	)
	if strings.Contains(src, "if read.Name.IsUnknown()") {
		t.Error("a readable attribute got the write-only unknown guard")
	}
}

func TestWriteOnlyReplaceSkipsEmptyState(t *testing.T) {
	cfg, r := writeOnlyFixture()
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatal(err)
	}
	src, reg := string(files[0].Content), string(files[len(files)-1].Content)
	wantAll(t, src,
		`"password":      schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, Description: "", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIf(replaceIfWriteOnlyRecorded, `,
		// Readable create-time attributes keep the plain RequiresReplace.
		`"name":          schema.StringAttribute{Required: true, Description: "", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}`,
	)
	wantAll(t, reg,
		"func replaceIfWriteOnlyRecorded(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {\n\tresp.RequiresReplace = req.StateValue.ValueString() != \"\"\n}",
	)
}

func TestRegistrationWithoutWriteOnlyHasNoHelper(t *testing.T) {
	cfg, r := typedFixture()
	files, err := Generate(cfg, []Resource{r})
	if err != nil {
		t.Fatal(err)
	}
	if reg := string(files[len(files)-1].Content); strings.Contains(reg, "replaceIfWriteOnlyRecorded") || strings.Contains(reg, `"context"`) {
		t.Errorf("registration file has the write-only helper without a write-only attribute:\n%s", reg)
	}
}
