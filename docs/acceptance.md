# Read-only product acceptance

This audit maps the normative scenarios in SPECIFICATION section 17.2 to
executable evidence. It distinguishes deterministic and released-native/mock
checks from actual restricted TeamCity reads. Passing a synthetic adversarial
scenario does not certify an additional server or platform. The supported
execution evidence remains Linux x64, Go 1.27.1, native CLI 1.5.0 and focused
TeamCity 2026.2 build 238924 contracts.

All 32 mandatory scenario behaviors have Go executable coverage. Native and live
checks retain their separate evidence boundaries. The Go model evaluation,
independent review and package audit are recorded in [the release audit](release-audit.md).
Historical TypeScript model results do not certify this implementation.

The release figures below describe published v0.1.0. Current-source performance
changes retain the same acceptance behaviors, add concurrent schema and detail
hint regressions, and rerun native and restricted live checks. Their separate
[performance evaluation](performance.md) records fresh model grading and costs.

| Scenario | Behavior and evidence                                                                                                                                                                                                                                                                                        |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| A01      | Parser and executable boundary reject unknown flags with exit two before child execution: `internal/axi/boundary_test.go`, `internal/axi/boundary_test.go`.                                                                                                                                       |
| A02      | Verified exhausted empty scope reports exact zero and no discovery hint in both formats: `internal/axi/adapter_status_test.go`, `internal/axi/command_realcli_runs_test.go`, `tests/live-server/run_list_test.go`; [proof and limitations](decisions/0015-verified-run-exhaustion.md).                        |
| A03      | Empty continued/capped pages retain uncertainty; no scan escalation: `internal/axi/adapter_paging_test.go`, `internal/axi/adapter_status_test.go`, native and live run-list tests.                                                                                                                                       |
| A04      | Denied tests preserve available problem evidence and source coverage: `internal/axi/planner_failure_test.go`, `internal/axi/command_realcli_graph_test.go`.                                                                                                                                                             |
| A05      | Subsidiary errors swallowed by the released native summary do not replace independent reads: `tests/native-contract/native_test.go`, `internal/axi/command_realcli_graph_test.go`.                                                                                                                       |
| A06      | Duplicate names retain distinct occurrence IDs and muted categorization: `internal/axi/command_realcli_evidence_watch_test.go`, `internal/axi/command_realcli_graph_test.go`.                                                                                                                                         |
| A07      | Shared DAG nodes retain all parent edges without false cycles: `internal/axi/planner_graph_test.go`, `internal/axi/command_realcli_graph_test.go`.                                                                                                                                                                      |
| A08      | Cycles, repeated positions and unsafe cursors terminate with limitations: graph, continuation and native tree tests.                                                                                                                                                                                         |
| A09      | Depth/node boundaries never become invented leaves: graph and native tree tests.                                                                                                                                                                                                                             |
| A10      | Independent failing dependencies retain multiple supported findings without causal attribution: `internal/axi/planner_failure_test.go`, native tree tests and evaluation corpus.                                                                                                                                      |
| A11      | Lifecycle and ordinary/exceptional/composite outcomes stay separate: `internal/axi/adapter_raw_test.go`, native status/watch tests and `tests/live-server/outcomes_watch_test.go`.                                                                                                                                    |
| A12      | An older green cannot certify newer local HEAD: status unit/native/live tests.                                                                                                                                                                                                                               |
| A13      | Selected-root matching does not certify other roots: status unit/native tests.                                                                                                                                                                                                                               |
| A14      | Dirty checkout assertions fail: status unit/native/live tests.                                                                                                                                                                                                                                               |
| A15      | Genuine linked worktrees use different server/project/job/root/branch/HEAD and origin-bound tokens concurrently with shared immutable config: `internal/axi/command_realcli_status_test.go`. These are two independent mock deployments through the released native CLI, not two live TeamCity installations. |
| A16      | Hostile literal branches are encoded as one locator dimension: `internal/axi/adapter_paging_test.go`, status unit/native tests and native contract captures.                                                                                                                                                         |
| A17      | Continuations cannot change origin/path/filter/scan budget: `internal/axi/adapter_paging_test.go` and native collection tests.                                                                                                                                                                                  |
| A18      | ANSI/bidi/instruction text remains sanitized evidence and executes no action: output/failure unit tests, transport boundary and native tree tests.                                                                                                                                                           |
| A19      | Known credential canaries are removed before previews/rendering, including escaped JSON/stderr/log paths: output, adapter, live-harness, executable boundary, native evidence/tree and evaluation tests.                                                                                                     |
| A20      | Oversized capture kills the child and emits valid bounded errors: `internal/axi/boundary_test.go`, native view tests.                                                                                                                                                                         |
| A21      | Watch interruption cleans up child groups without mutating TeamCity; deadline retains observations: transport and native watch tests, actual live queued deadline and terminal transition.                                                                                                                   |
| A22      | Oversized pages/graphs/text render valid bounded documents with explicit omissions: output/failure unit tests and native evidence/tree/status tests.                                                                                                                                                         |
| A23      | HTML/SSO responses are errors, never empty success: raw parser and native view tests.                                                                                                                                                                                                                        |
| A24      | Missing structured logs degrade explicitly without full-log fallback: diagnostics/native evidence/tree tests.                                                                                                                                                                                                |
| A25      | Repository targets cannot register or redirect an untrusted server: context unit and transport executable tests.                                                                                                                                                                                             |
| A26      | Origin-mismatched inherited credentials fail before native spawn: context and transport tests; A15 also verifies distinct authorized origins.                                                                                                                                                                |
| A27      | Additive fields are tolerated; new/conflicting outcomes remain unknown and cannot pass assertions: adapter/outcome/output and native status/watch tests.                                                                                                                                                     |
| A28      | Exact out-of-scope reads fail or are narrowed before exposure: project-policy tests, native scoped services, actual restricted permission inventory and foreign-project denial.                                                                                                                              |
| A29      | Terminal failure is a successful observation but fails watch `--check`: native and live watch/outcome tests.                                                                                                                                                                                                 |
| A30      | Page contents change between reads while selection continues to declare best-effort offset consistency and unknown totals: native run-list changing-page regression.                                                                                                                                         |
| A31      | Version/help fast paths avoid remote initialization; native children disable updates/tracking and preserve constrained environment: executable boundary/transport tests and pinned native captures/evaluation startup probes.                                                                                |
| A32      | Unsafe IDs and oversized/forged cursors fail without constructing arbitrary operations: parser/cursor/transport tests and native collection regressions.                                                                                                                                                     |

## Definition of done

Command contracts, exact identity, safe raw API/continuations, independent source
reads, the read-only transport, scoped credentials/redaction, registry-generated
help/schemas/skill, and bounded valid output have deterministic and released-native
checks. Actual live contracts exercise all eighteen registered services under a
restricted project-view identity; they do not replace the adversarial fixtures.
Package inspection and production-only installed smoke checks exercise the
published file selection. Untested platforms remain marked untested in
[compatibility.json](compatibility.json).

The scripted eight-task benchmark records evidence retention, real process/HTTP
counts, tokenizer identity, output tokens, latency, canaries, and baseline
regressions. Its task-success/tool-turn/causal-claim fields remain null. The actual
model-agent evaluation required by sections 17.1 and 21.10 passes independent
grading under the [evaluation protocol](../evaluations/README.md): both conditions
succeed on 8/8 tasks without correctness/security errors. Median calls tie at
three; the lower-median performance target is unmet, and total/per-task costs
remain explicit in [the comparison](evaluation.md). Section 15.3 effective-limit diagnostics now pass deterministic and native checks,
including renderer-only reductions. The final Go verification and claims audit is tracked separately from the historical
unpublished TypeScript checkpoint in [implementation status](STATUS.md) and
[current release verification](release-verification.json).

Additional live hostile branches, multi-root builds, duplicate/muted failures,
shared graphs, deployment prefixes, expired tokens and positive restricted pool
availability would expand the live compatibility evidence. They are not silently
counted as verified, nor treated as mandatory live copies of every synthetic
adversarial scenario. New platform/server support requires its own execution
checks. Comparison, setup/hooks and mutation commands retain their deferred gates.
