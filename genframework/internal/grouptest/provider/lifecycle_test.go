package provider

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/terraprovider/go-msadmin/consistency"
	"github.com/terraprovider/tf-msadmin/resourcex"

	"github.com/terraprovider/tf-msadmin/genframework/internal/grouptest/clients"
	"github.com/terraprovider/tf-msadmin/genframework/internal/grouptest/cs"
)

const (
	g1 = "11111111-1111-1111-1111-111111111111"
	g2 = "22222222-2222-2222-2222-222222222222"
	g3 = "33333333-3333-3333-3333-333333333333"
)

type harness struct {
	t   *testing.T
	ctx context.Context
	r   *meetingPolicyGroupAssignmentResource
	svc *cs.Service
	s   schema.Schema
	// diags of the last create/update
	diags diag.Diagnostics
}

func newHarness(t *testing.T, lag int) *harness {
	old := meetingPolicyGroupAssignmentResourceWait
	meetingPolicyGroupAssignmentResourceWait = consistency.Config{Attempts: 10, Delay: time.Millisecond}
	t.Cleanup(func() { meetingPolicyGroupAssignmentResourceWait = old })
	h := &harness{t: t, ctx: context.Background(), r: &meetingPolicyGroupAssignmentResource{}, svc: &cs.Service{Lag: lag}}
	h.r.Configure(h.ctx, resource.ConfigureRequest{ProviderData: &clients.Client{CS: h.svc}}, &resource.ConfigureResponse{})
	var sr resource.SchemaResponse
	h.r.Schema(h.ctx, resource.SchemaRequest{}, &sr)
	h.s = sr.Schema
	return h
}

// value builds an object value; a nil field is null (id before create, import).
func (h *harness) value(id, group, name *string, rank *int64) tftypes.Value {
	typ := h.s.Type().TerraformType(h.ctx)
	str := func(p *string) tftypes.Value {
		if p == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *p)
	}
	num := tftypes.NewValue(tftypes.Number, nil)
	if rank != nil {
		num = tftypes.NewValue(tftypes.Number, new(big.Float).SetInt64(*rank))
	}
	return tftypes.NewValue(typ, map[string]tftypes.Value{"id": str(id), "group_id": str(group), "policy_name": str(name), "rank": num})
}

func ptr[T any](v T) *T { return &v }

func (h *harness) model(st tfsdk.State) meetingPolicyGroupAssignmentModel {
	h.t.Helper()
	var m meetingPolicyGroupAssignmentModel
	if d := st.Get(h.ctx, &m); d.HasError() {
		h.t.Fatalf("state: %v", d)
	}
	return m
}

func (h *harness) create(group, name string, rank int64) (tfsdk.State, string) {
	h.t.Helper()
	resp := resource.CreateResponse{State: tfsdk.State{Schema: h.s, Raw: h.value(nil, nil, nil, nil)}}
	h.r.Create(h.ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: h.s, Raw: h.value(nil, &group, &name, &rank)}}, &resp)
	h.diags = resp.Diagnostics
	return resp.State, errs(resp.Diagnostics)
}

func (h *harness) update(st tfsdk.State, name string, rank int64) (tfsdk.State, string) {
	h.t.Helper()
	m := h.model(st)
	id, group := m.ID.ValueString(), m.GroupID.ValueString()
	resp := resource.UpdateResponse{State: st}
	h.r.Update(h.ctx, resource.UpdateRequest{State: st, Plan: tfsdk.Plan{Schema: h.s, Raw: h.value(&id, &group, &name, &rank)}}, &resp)
	h.diags = resp.Diagnostics
	return resp.State, errs(resp.Diagnostics)
}

// warned reports whether the last create/update returned a warning with this summary.
func (h *harness) warned(summary string) bool {
	for _, w := range h.diags.Warnings() {
		if w.Summary() == summary {
			return true
		}
	}
	return false
}

func (h *harness) read(st tfsdk.State) tfsdk.State {
	h.t.Helper()
	resp := resource.ReadResponse{State: st}
	h.r.Read(h.ctx, resource.ReadRequest{State: st}, &resp)
	if e := errs(resp.Diagnostics); e != "" {
		h.t.Fatalf("read: %s", e)
	}
	return resp.State
}

func (h *harness) delete(st tfsdk.State) string {
	resp := resource.DeleteResponse{State: st}
	h.r.Delete(h.ctx, resource.DeleteRequest{State: st}, &resp)
	return errs(resp.Diagnostics)
}

// errs joins the error diagnostics ("" when there are none).
func errs(d diag.Diagnostics) string {
	var out []string
	for _, e := range d.Errors() {
		out = append(out, e.Summary()+": "+e.Detail())
	}
	return strings.Join(out, "; ")
}

func (h *harness) want(st tfsdk.State, name string, rank int64) {
	h.t.Helper()
	m := h.model(st)
	if m.PolicyName.ValueString() != name || m.Rank.ValueInt64() != rank || m.ID.ValueString() != m.GroupID.ValueString() {
		h.t.Fatalf("state = (%q, %d, id %q), want (%q, %d, id = group_id)", m.PolicyName.ValueString(), m.Rank.ValueInt64(), m.ID.ValueString(), name, rank)
	}
}

func TestLifecycle(t *testing.T) {
	h := newHarness(t, 2) // every write surfaces after 2 stale reads

	a, e := h.create(g1, "Foo", 1)
	if e != "" {
		t.Fatal(e)
	}
	h.want(a, "Foo", 1)

	// Another group inserted at rank 1 renumbers g1: Read reports it as drift.
	b, e := h.create(g2, "Bar", 1)
	if e != "" {
		t.Fatal(e)
	}
	a = h.read(a)
	h.want(a, "Foo", 2)

	// Apply restores the configured rank (and pushes g2 down).
	if a, e = h.update(a, "Foo", 1); e != "" {
		t.Fatal(e)
	}
	h.want(a, "Foo", 1)
	h.want(h.read(b), "Bar", 2)

	// Rename in place; a case-only difference from Teams keeps the configured spelling.
	if a, e = h.update(a, "Baz", 1); e != "" {
		t.Fatal(e)
	}
	h.svc.Set(g1, "BAZ", 1)
	h.want(h.read(a), "Baz", 1)

	// External deletion removes the resource from state.
	h.svc.Set(g2, "", 0)
	if st := h.read(b); !st.Raw.IsNull() {
		t.Fatalf("externally deleted assignment must be removed from state, got %v", st.Raw)
	}

	// Destroy waits for the removal and returns once it is confirmed by
	// resourcex.ReflectedReads consecutive reads: the getter/predicate
	// contradiction in #4 burned the whole retry budget instead.
	before := h.svc.Reads
	if e := h.delete(a); e != "" {
		t.Fatal(e)
	}
	if n, want := h.svc.Reads-before, h.svc.Lag+resourcex.ReflectedReads; n != want {
		t.Fatalf("delete polled %d times, want %d (lag + consecutive reads)", n, want)
	}
	if name, _, _ := h.svc.GroupPolicyAssignment(h.ctx, g1, "TeamsMeetingPolicy"); name != "" {
		t.Fatalf("g1 still assigned %q after delete", name)
	}
}

// Teams places a rank beyond the number of assignments last (live-verified).
// The apply warns instead of failing, and the next refresh shows the drift.
func TestRankPlacedLast(t *testing.T) {
	h := newHarness(t, 0)
	if _, e := h.create(g1, "Foo", 1); e != "" {
		t.Fatal(e)
	}
	before := h.svc.Reads
	b, e := h.create(g2, "Bar", 5)
	if e != "" || !h.warned("Rank placed last") {
		t.Fatalf("want a rank warning and no error, got error %q, warnings %v", e, h.diags.Warnings())
	}
	// The placed-last read differs from the pre-grant one, so the wait ends
	// early: one pre-grant read plus the consecutive reads.
	if n, want := h.svc.Reads-before, 1+resourcex.ReflectedReads; n != want {
		t.Fatalf("create read %d times, want %d", n, want)
	}
	h.want(h.read(b), "Bar", 2)
}

// Terraform picks the order of independent creates; the lock only serializes
// them. A higher rank created first is placed last, and once the lower rank
// exists the refresh matches the configuration.
func TestCreateOrder(t *testing.T) {
	h := newHarness(t, 2)
	b, e := h.create(g2, "Bar", 2)
	if e != "" || !h.warned("Rank placed last") {
		t.Fatalf("rank 2 first: want a warning and no error, got %q", e)
	}
	a, e := h.create(g1, "Foo", 1)
	if e != "" || h.warned("Rank placed last") {
		t.Fatalf("rank 1 second: got error %q, warnings %v", e, h.diags.Warnings())
	}
	h.want(h.read(a), "Foo", 1)
	h.want(h.read(b), "Bar", 2)
}

// Moving an assignment down within range: while reads still return the
// pre-grant state, the old lower rank must not be taken for "placed last".
func TestMoveDownWithLag(t *testing.T) {
	h := newHarness(t, 2)
	a, _ := h.create(g1, "Foo", 1)
	h.create(g2, "Bar", 2)
	h.create(g3, "Baz", 3)
	a, e := h.update(a, "Foo", 3)
	if e != "" || h.warned("Rank placed last") {
		t.Fatalf("want no error and no warning, got error %q, warnings %v", e, h.diags.Warnings())
	}
	h.want(h.read(a), "Foo", 3)
}

// Removing the lower-ranked assignment leaves the other at rank 1. Re-applying
// its unchanged rank 2 reads the same as before the grant, which cannot be
// told apart from a grant that never propagated, so it fails and asks for a
// reachable rank.
func TestReapplyUnreachableRank(t *testing.T) {
	h := newHarness(t, 0)
	a, _ := h.create(g1, "Foo", 1)
	b, _ := h.create(g2, "Bar", 2)
	if e := h.delete(a); e != "" {
		t.Fatal(e)
	}
	b = h.read(b)
	h.want(b, "Bar", 1)
	if _, e := h.update(b, "Bar", 2); !strings.Contains(e, "cannot be reached") {
		t.Fatalf("want an unreachable-rank error, got %q", e)
	}
}

// A placed-last read must also hold on consecutive reads: one at the very end
// of the budget is not enough.
func TestPlacedLastNeedsConsecutiveReads(t *testing.T) {
	h := newHarness(t, 0)
	h.create(g1, "Foo", 1)
	h.svc.Lag = meetingPolicyGroupAssignmentResourceWait.Attempts - 1
	if _, e := h.create(g2, "Bar", 5); !strings.Contains(e, "did not reach the requested state") {
		t.Fatalf("want a did-not-reach error, got %q", e)
	}
}

// An in-range move whose grant never shows up fails; the unchanged lower rank
// is not taken for "placed last".
func TestMoveDownNeverVisible(t *testing.T) {
	h := newHarness(t, 0)
	a, _ := h.create(g1, "Foo", 1)
	h.create(g2, "Bar", 2)
	h.create(g3, "Baz", 3)
	h.svc.Lag = 1000
	if _, e := h.update(a, "Foo", 3); !strings.Contains(e, "did not reach the requested state") || h.warned("Rank placed last") {
		t.Fatalf("want a did-not-reach error and no warning, got %q, warnings %v", e, h.diags.Warnings())
	}
}

// Two different states that each satisfy the wait (the requested rank, then a
// lower one that looks placed last) must not add up to one confirmation: each
// state needs its own consecutive reads.
func TestMixedOutcomesDoNotConfirm(t *testing.T) {
	h := newHarness(t, 0)
	a, _ := h.create(g1, "Foo", 1)
	h.create(g2, "Bar", 2)
	h.create(g3, "Baz", 3)
	h.svc.AfterGrant = []cs.Read{{Name: "Foo", Rank: 3}, {Name: "Foo", Rank: 2}}
	before := h.svc.Reads
	a, e := h.update(a, "Foo", 3)
	if e != "" || h.warned("Rank placed last") {
		t.Fatalf("want no error and no warning, got error %q, warnings %v", e, h.diags.Warnings())
	}
	// pre-grant read + the two scripted reads + two consecutive reads of rank 3
	if n, want := h.svc.Reads-before, 1+2+resourcex.ReflectedReads; n != want {
		t.Fatalf("update read %d times, want %d", n, want)
	}
	h.want(h.read(a), "Foo", 3)
}

// A stale read that differs from the pre-grant read only in the name's case
// is still stale, not "placed last".
func TestStaleReadCaseOnly(t *testing.T) {
	h := newHarness(t, 0)
	a, _ := h.create(g1, "Foo", 1)
	h.create(g2, "Bar", 2)
	h.create(g3, "Baz", 3)
	h.svc.StaleUpper = true
	h.svc.Lag = 2
	a, e := h.update(a, "Foo", 3)
	if e != "" || h.warned("Rank placed last") {
		t.Fatalf("want no error and no warning, got error %q, warnings %v", e, h.diags.Warnings())
	}
	h.want(h.read(a), "Foo", 3)
}

// A grant that never shows up fails the create, but the state is saved so
// Terraform taints the resource instead of losing track of the grant.
func TestCreateWaitFailsKeepsState(t *testing.T) {
	h := newHarness(t, 1000)
	st, e := h.create(g1, "Foo", 1)
	if !strings.Contains(e, "did not reach the requested state") {
		t.Fatalf("want a did-not-reach error, got %q", e)
	}
	if st.Raw.IsNull() {
		t.Fatal("state must be saved after a successful grant, so Terraform taints the resource")
	}
	h.want(st, "Foo", 1)
}

// A failed update keeps the prior state.
func TestUpdateWaitFails(t *testing.T) {
	h := newHarness(t, 0)
	a, _ := h.create(g1, "Foo", 1)
	h.svc.Lag = 1000
	st, e := h.update(a, "Baz", 1)
	if !strings.Contains(e, "did not reach the requested state") {
		t.Fatalf("want a did-not-reach error, got %q", e)
	}
	h.want(st, "Foo", 1)
}

// The wait needs resourcex.ReflectedReads consecutive matching reads: one
// matching read at the very end of the budget is not enough.
func TestSingleMatchAtOverrun(t *testing.T) {
	h := newHarness(t, 0)
	h.svc.Lag = meetingPolicyGroupAssignmentResourceWait.Attempts - 1
	if _, e := h.create(g1, "Foo", 1); !strings.Contains(e, "did not reach the requested state") {
		t.Fatalf("want a did-not-reach error, got %q", e)
	}
}

func TestImport(t *testing.T) {
	h := newHarness(t, 0)
	h.svc.Set(g3, "Foo", 1)
	resp := resource.ImportStateResponse{State: tfsdk.State{Schema: h.s, Raw: h.value(nil, nil, nil, nil)}}
	h.r.ImportState(h.ctx, resource.ImportStateRequest{ID: g3}, &resp)
	if e := errs(resp.Diagnostics); e != "" {
		t.Fatal(e)
	}
	h.want(h.read(resp.State), "Foo", 1)
}

func TestValidateConfig(t *testing.T) {
	h := newHarness(t, 0)
	for name, tc := range map[string]struct {
		group, policy string
		rank          int64
		wantErr       string
	}{
		"valid":      {g1, "Foo", 1, ""},
		"not a uuid": {"sales", "Foo", 1, "Invalid group_id"},
		"empty name": {g1, "", 1, "Invalid policy_name"},
		"rank zero":  {g1, "Foo", 0, "Invalid rank"},
	} {
		var resp resource.ValidateConfigResponse
		h.r.ValidateConfig(h.ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: h.s, Raw: h.value(nil, &tc.group, &tc.policy, &tc.rank)}}, &resp)
		if got := errs(resp.Diagnostics); (tc.wantErr == "") != (got == "") || !strings.Contains(got, tc.wantErr) {
			t.Errorf("%s: got %q, want %q", name, got, tc.wantErr)
		}
	}
}

// TestLockPerTenant: the policy-type lock is keyed by tenant, so an alias for
// another tenant is not blocked while one tenant's write holds it.
func TestLockPerTenant(t *testing.T) {
	res := func(tenant string) *meetingPolicyGroupAssignmentResource {
		return &meetingPolicyGroupAssignmentResource{client: &clients.Client{CS: &cs.Service{}, TenantID: tenant}}
	}
	unlock := res("tenant-a").lock()
	acquired := func(r *meetingPolicyGroupAssignmentResource) bool {
		done := make(chan struct{})
		go func() { r.lock()(); close(done) }()
		select {
		case <-done:
			return true
		case <-time.After(50 * time.Millisecond):
			return false
		}
	}
	if !acquired(res("tenant-b")) {
		t.Fatal("tenant-b blocked by tenant-a's lock")
	}
	if acquired(res("tenant-a")) {
		t.Fatal("second tenant-a writer was not serialized")
	}
	unlock() // releases the blocked tenant-a goroutine
}
