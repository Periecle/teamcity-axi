package axi

import (
	"context"
	"testing"
)

type adapterPolicyReader struct{ read func(ReadRequest) ReadResult }

func (r adapterPolicyReader) Read(_ context.Context, q ReadRequest, _ Budget) ReadResult {
	return r.read(q)
}
func TestAdapterPolicyObservedAncestorsCacheCycleDepthAndUnavailable(t *testing.T) {
	calls := []string{}
	reader := adapterPolicyReader{read: func(q ReadRequest) ReadResult {
		calls = append(calls, q.ID)
		parent := "_Root"
		if q.ID == "Child" {
			parent = "Allowed"
		}
		return ReadResult{State: "available", Value: Object{"id": q.ID, "parentProjectId": parent}}
	}}
	policy := NewProjectPolicy(reader, []string{"Allowed"}, adapterBudget())
	ctx := context.Background()
	adapterOK(t, policy.Assert(ctx, "Child"))
	adapterOK(t, policy.Assert(ctx, "Child"))
	adapterWant(t, calls, []string{"Child"})
	adapterError(t, policy.Assert(ctx, "Allowed_Evil"), "POLICY_DENIED")
	adapterError(t, policy.Assert(ctx, ""), "POLICY_DENIED")
	cycle := NewProjectPolicy(adapterPolicyReader{read: func(q ReadRequest) ReadResult {
		parent := "a"
		if q.ID == "a" {
			parent = "b"
		}
		return ReadResult{State: "available", Value: Object{"id": q.ID, "parentProjectId": parent}}
	}}, []string{"Allowed"}, adapterBudget())
	adapterError(t, cycle.Assert(ctx, "a"), "POLICY_DENIED")
	depth := 0
	deep := NewProjectPolicy(adapterPolicyReader{read: func(q ReadRequest) ReadResult {
		depth++
		return ReadResult{State: "available", Value: Object{"parentProjectId": strconvInt(depth)}}
	}}, []string{"Allowed"}, adapterBudget())
	adapterError(t, deep.Assert(ctx, "first"), "POLICY_DENIED")
	adapterWant(t, depth, 8)
	unavailable := NewProjectPolicy(adapterPolicyReader{read: func(ReadRequest) ReadResult {
		return ReadResult{State: "unavailable", Error: NewError("PERMISSION_DENIED", "denied", 1)}
	}}, []string{"Allowed"}, adapterBudget())
	adapterError(t, unavailable.Assert(ctx, "Other"), "PERMISSION_DENIED")
}
func TestAdapterPolicyAcceptsEighthObservedAncestor(t *testing.T) {
	count := 0
	policy := NewProjectPolicy(adapterPolicyReader{read: func(q ReadRequest) ReadResult {
		count++
		return ReadResult{State: "available", Value: Object{"id": q.ID, "parentProjectId": "p" + strconvInt(count)}}
	}}, []string{"p8"}, adapterBudget())
	adapterOK(t, policy.Assert(context.Background(), "p0"))
	adapterWant(t, count, 8)
}
