package axi

import "testing"

func plannerStatusSnapshot(runs []Object, notes []Limitation) Object {
	return Object{"job": Object{"id": "Job"}, "page": Object{"runs": runs, "providerReturned": len(runs), "position": nil, "hasMore": nil, "limitations": notes}}
}

func plannerCheckoutRun(id int, overrides Object) Object {
	run := plannerRun(id, Object{"jobId": "Job", "result": "success", "personal": false, "revisions": []Object{{"vcsRootId": "Root", "revision": "exact"}}})
	for key, value := range overrides {
		run[key] = value
	}
	return run
}

func plannerAssess(runs []Object) Object {
	return MatchCheckout(ExecutionContext{Job: "Job", Revision: "exact", VCSRootID: "Root"}, plannerStatusSnapshot(runs, nil))
}

func TestPlannerStatusNewestExactOutcome(t *testing.T) {
	for _, result := range []string{"failure", "error", "canceled", "failed_to_start", "success"} {
		t.Run(result, func(t *testing.T) {
			value := plannerAssess([]Object{plannerCheckoutRun(2, Object{"result": result}), plannerCheckoutRun(1, nil)})
			expected := "failed"
			if result == "success" {
				expected = "passed"
			}
			if Str(value, "match") != "exact" || Str(value, "assessment") != expected || Str(Obj(value["run"]), "id") != "2" {
				t.Fatalf("newest exact outcome: %#v", value)
			}
		})
	}
}

func TestPlannerStatusActiveCandidatesBlockOlderGreen(t *testing.T) {
	for _, state := range []string{"queued", "running"} {
		value := plannerAssess([]Object{plannerCheckoutRun(2, Object{"state": state}), plannerCheckoutRun(1, nil)})
		if Str(value, "assessment") != "in_progress" || Str(Obj(value["run"]), "id") != "2" || Int(Obj(value["activity"]), state) != 1 || Str(Obj(value["activity"]), "scope") != "returnedCandidates" {
			t.Fatalf("active exact checkout: %#v", value)
		}
	}
}

func TestPlannerStatusDifferentAndUnknownNewerRevision(t *testing.T) {
	older := plannerCheckoutRun(1, nil)
	different := plannerCheckoutRun(2, Object{"revisions": []Object{{"vcsRootId": "Root", "revision": "different"}}})
	if value := plannerAssess([]Object{different, older}); Str(value, "assessment") != "passed" || Str(Obj(value["run"]), "id") != "1" {
		t.Fatalf("different checkout blocked exact: %#v", value)
	}
	unknown := plannerAssess([]Object{plannerCheckoutRun(2, Object{"revisions": []Object{}}), older})
	if Str(unknown, "match") != "exact" || Str(unknown, "assessment") != "unverified" || Str(Obj(unknown["run"]), "id") != "1" || !plannerHasLimitation(plannerLimitations(unknown["limitations"]), "NEWER_REVISION_UNVERIFIED") {
		t.Fatalf("newer unknown certified old green: %#v", unknown)
	}
}

func TestPlannerStatusMissingAndAmbiguousRevision(t *testing.T) {
	for _, value := range []Object{plannerAssess(nil), plannerAssess([]Object{plannerCheckoutRun(1, Object{"revisions": []Object{}})}), AssessStatusJob("Missing", nil, "exact", "Root"), AssessStatusJob("Job", plannerStatusSnapshot([]Object{plannerCheckoutRun(1, nil)}, nil), "", "Root"), plannerAssess([]Object{plannerCheckoutRun(1, Object{"revisions": []Object{{"vcsRootId": "Root", "revision": "exact"}, {"vcsRootId": "Root", "revision": "exact"}}})})} {
		if Str(value, "match") != "unverified" || Str(value, "assessment") != "unverified" || len(plannerLimitations(value["limitations"])) == 0 {
			t.Fatalf("unverified revision certified: %#v", value)
		}
	}
	stale := plannerAssess([]Object{plannerCheckoutRun(1, Object{"revisions": []Object{{"vcsRootId": "Root", "revision": "different"}}})})
	if Str(stale, "match") != "different" || Str(stale, "assessment") != "unverified" {
		t.Fatalf("stale revision: %#v", stale)
	}
}

func TestPlannerStatusOtherRootsAndPersonalCheckouts(t *testing.T) {
	multi := plannerAssess([]Object{plannerCheckoutRun(1, Object{"revisions": []Object{{"vcsRootId": "Root", "revision": "exact"}, {"vcsRootId": "Other", "revision": "different"}}})})
	if Str(multi, "match") != "exact" || Str(multi, "assessment") != "unverified" || len(Objects(Obj(multi["run"])["revisions"])) != 2 || !plannerHasLimitation(plannerLimitations(multi["limitations"]), "OTHER_ROOTS_UNVERIFIED") {
		t.Fatalf("multi-root checkout certified: %#v", multi)
	}
	for _, personal := range []any{true, nil} {
		value := plannerAssess([]Object{plannerCheckoutRun(1, Object{"personal": personal})})
		if Str(value, "assessment") != "unverified" || !plannerHasLimitation(plannerLimitations(value["limitations"]), "PERSONAL_CHECKOUT_UNVERIFIED") {
			t.Fatalf("personal checkout certified: %#v", value)
		}
	}
}

func TestPlannerStatusUnsafeContinuationAndUnknownOutcome(t *testing.T) {
	value := AssessStatusJob("Job", plannerStatusSnapshot([]Object{plannerCheckoutRun(1, nil)}, []Limitation{{Code: "UNSAFE_CONTINUATION", Message: "Unsafe cursor"}}), "exact", "Root")
	if Str(value, "match") != "exact" || Str(value, "assessment") != "unverified" || !plannerHasLimitation(plannerLimitations(value["limitations"]), "UNSAFE_CONTINUATION") {
		t.Fatalf("unsafe page certified: %#v", value)
	}
	value = plannerAssess([]Object{plannerCheckoutRun(1, Object{"result": "unknown"})})
	if Str(value, "assessment") != "unverified" || !plannerHasLimitation(plannerLimitations(value["limitations"]), "RUN_OUTCOME_UNVERIFIED") {
		t.Fatalf("unknown outcome certified: %#v", value)
	}
}
