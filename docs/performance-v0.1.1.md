# Go v0.1.1 performance and release

v0.1.1 removes redundant local work while preserving the read-only v0.1 contract.
It includes the earlier post-release schema/hint improvements and adds renderer,
sanitizer, text-boundary, environment and portable-guidance changes. The previous
release and its reports remain available at v0.1.0; the earlier optimized source
comparison is retained in [performance.md](performance.md).

## Why the historical TS model session was faster

Historical TS completed the model sample at a 37.3-second median, released Go at
55.4 seconds, and the first optimized Go checkpoint at 44.3 seconds. Those are
whole model sessions, including reasoning and protocol round trips. They do not
measure only the implementation language. Scripted TOON workflows measured
303 ms in TS, 111 ms in released Go, and 103 ms in the first optimized Go sample.
Median cumulative tool execution in the optimized model sample was 92 ms; total
tool execution was 0.28% of total session time. Native-control session medians
also changed substantially between runs. Different full prompts, scheduling and
model backend state prevent a controlled language comparison.

The old Go traces contained help/schema discovery, repeated reads and unnecessary
expansion. Lazy schema compilation, precise truncation hints and first-read
commands addressed these observable causes: total model calls fell 30 to eight,
with one call per wrapper task, while all eight tasks remained successful.
Wrapper output was still larger than native output, and historical TS session
latency was still lower. These limitations are retained rather than dismissed.

The fresh v0.1.1 sample also spends only 0.282% of total wrapper session time
executing tools, with a 118 ms median cumulative tool time. Thus even removing
all measured tool time would save less than one percent of total observed session
time. The traces establish the cost split; they do not identify a provider-side
cause for the historical TS timing advantage.

## Additional changes in v0.1.1

- Share one detached JSON tree between initial envelope validation, payload
  validation and serialization, removing two repeated conversions on that path.
  Keep canonical number handling, omitempty, sorted JSON keys and both gates.
- Avoid allocating a text builder for clean valid UTF-8. Skip ANSI regex work
  when no escape is present, and replacement allocation when the exact secret
  regex does not match. Preserve both known-secret passes and all regex checks.
- Replace copied sanitizer path slices with equivalent root/meta protection
  state. No global output or secret cache is added.
- Bound excerpts at rune byte offsets, without materializing whole rune slices.
  Reuse the actual excerpt truncation result rather than sanitizing/counting the
  same input again. Fingerprint inputs and exact retrieval scopes are unchanged.
- Compile fixed header/version regexes once. Snapshot the command environment
  once for context and public rendering, and reuse its known secrets on fallback.
  Native session acquisition still performs its own version and authority checks.
- Reduce portable guidance from 6,721 to 4,625 bytes (31%), keeping first-read
  syntax, item/source identities, unknown/partial semantics, status/watch rules,
  untrusted-evidence handling and trusted configuration boundaries. The task
  corpus, task prompts, model effort, safety caps and grading are unchanged.

## In-process measurements

Three samples per benchmark and condition use Go 1.27.1, 150 ms per sample,
on the same Linux amd64 host. Baseline product source is `6f7a1f2`; the new
benchmark harness also runs against that baseline. All initial candidate samples
are retained. See [raw and parsed measurements](../evaluations/results/v0.1.1/microbenchmarks.json).

| Renderer | Baseline | v0.1.1 | Allocations before → after |
| --- | ---: | ---: | ---: |
| Exact status, JSON | 535 µs | 388 µs | 3291 → 1420 |
| Exact status, TOON | 619 µs | 436 µs | 3627 → 1754 |
| Partial failure, JSON | 968 µs | 629 µs | 6715 → 2697 |
| Partial failure, TOON | 1074 µs | 746 µs | 7357 → 3338 |
| Compacted notes, JSON | 1700 µs | 1203 µs | 13181 → 3858 |
| Compacted notes, TOON | 1711 µs | 1249 µs | 13265 → 3941 |

Rendering falls 27–35% and allocation counts 52–71% in these cases. Plain ASCII
and Unicode sanitizer allocations fall 29/27 to zero per call, with observed
median time improvements of 11%/14%. Control-bearing secret text remains fully
sanitized. Sequential host samples are exploratory, not a performance guarantee
or proof of whole-session benefit.

## Remaining opportunities and scope limits

Model startup/reasoning/round trips dominate the retained traces; changing model
or effort requires a separately controlled quality/latency evaluation. Native
CLI startup and independent evidence HTTP requests remain a cost. Persistent
caching, long-lived workers or direct HTTP transport would require freshness,
authority, cleanup and compatibility work. Required reads cannot be removed
merely to make the wrapper cheaper than a partial native workflow.

Some actual sessions still batch failure and view, repeating the root metadata
read. Improving generic guidance or exposing additional accepted lifecycle fields
could reduce that work, but needs its own contract/quality check. One wrapper
model call per task is already the minimum for an evidence-based read; further
session gains cannot come from reducing that median below one.

Byte-budget reduction still serializes each retained optional-change prefix;
optimizing it requires exact-boundary and nonmonotonic-format evidence. Some
normalization and sanitizer passes are retained deliberately: sanitization is
not idempotent for arbitrary short secrets, and normalization detaches caller
objects and preserves JSON semantics. No unsafe global secret cache, weakened
schema validation, reduced required evidence or larger budgets were introduced.

The local savings and preserved behavior justify this release without CPU
profiling or more complex architecture. Output-token parity with native and a
statistically controlled whole-session TS comparison remain separate unmet
objectives unless new measurements explicitly establish them.

## Complete CLI workflow and startup observations

Two complete scripted pairs use reversed outer order and three internal
repetitions each. All 384 observations are retained. Each wrapper condition has
48/48 retained-evidence observations before and after, with zero identity,
completeness, secret or byte-budget errors. Required native process and HTTP
counts, and wrapper output-token counts, are unchanged. Native output-token
medians stay unchanged, while secret-task rows vary with fresh synthetic canary
values: selected JSON secret-task tokens per observation are 2116/2120 across
baseline runs versus 2124/2132 across candidate runs, and failure diagnostics
1040/1042 versus 1044/1048. Those rows remain retained.

| Workflow | Before | v0.1.1 | Median output tokens |
| --- | ---: | ---: | ---: |
| Wrapper TOON | 99.60 ms | 99.55 ms | 3004 |
| Wrapper JSON | 100.45 ms | 98.46 ms | 2699 |
| Native selected JSON control | 46.27 ms | 46.36 ms | 1009.5 |
| Native failure summary control | 13.22 ms | 13.23 ms | 238 |

JSON improves about 2%; TOON is effectively unchanged in this host sample. The
local renderer gains are a small part of workflows dominated by required native
processes and reads. Native selected JSON retains 42/48 observations and
exposes the synthetic canary in 12 calls per condition; standalone native failure
diagnostics retain 0/48 under this compound evidence rubric, with six exposing
calls. These are scoped evidence/safety limitations, not general CLI quality
ratings. The same limitations remain before and after.

Thirty alternating cold starts per binary/command retain all 180 observations:
version 5.75 → 5.80 ms, schema 15.78 → 15.32 ms, context 13.74 → 13.98 ms. This
is a clean configuration, PATH=/nonexistent, non-repository CWD measurement.
Context regresses slightly; no general startup improvement is claimed over the
already optimized baseline. The model evaluation ran concurrently with these
scripted/startup samples; required native/live/race suites had finished. All
samples and controls remain included.

See the [baseline](../evaluations/results/v0.1.1/scripted-baseline.json),
[candidate](../evaluations/results/v0.1.1/scripted-candidate.json),
[candidate repeat](../evaluations/results/v0.1.1/scripted-candidate-repeat.json),
[baseline repeat](../evaluations/results/v0.1.1/scripted-baseline-repeat.json) and
[startup](../evaluations/results/v0.1.1/startup.json) reports.

## Fresh complete model comparison

All sixteen fresh sessions completed, and a separate security/evidence reviewer
accepted every answer and full command/result/request trace. Both conditions
succeed on 8/8 tasks with zero identity, completeness, unsupported-causal,
output-secret or final-answer-secret errors. The fixed corpus, gpt-6.1-sol,
xhigh effort, alternating paired order, five-minute session cap and evidence
budgets are unchanged. Each task has one paired sample; backend weights are not
immutably identified and the measurements are not statistical guarantees.

| Model-session condition | Median time | Median calls | Total calls | Median output tokens |
| --- | ---: | ---: | ---: | ---: |
| Historical TS wrapper | 37.33 s | 3 | 30 | 4655 |
| Released Go v0.1.0 wrapper | 55.38 s | 3 | 30 | 5576.5 |
| Earlier optimized Go wrapper | 44.32 s | 1 | 8 | 3001.5 |
| Go v0.1.1 wrapper | 45.47 s | 1 | 9 | 3011 |
| Fresh native TeamCity CLI | 105.05 s | 2.5 | 24 | 1292 |

The fresh wrapper median is 17.9% lower than released Go and 56.7% lower than
the fresh native control. It remains 21.8% above historical TS and regresses
2.6% against the earlier optimized Go sample. Native medians also move from
historical TS's 85.22 s and the earlier Go sample's 135.28 s to 105.05 s here,
so these comparisons cannot isolate an implementation-language effect. The
smaller generic skill reduces median full wrapper prompt length from 7383 to
5287 characters; it does not demonstrate improved whole-session latency.

All task timing and call regressions remain visible:

| Wrapper task | Earlier optimized Go | v0.1.1 | Calls before → after |
| --- | ---: | ---: | ---: |
| Root failure | 40.43 s | 34.18 s | 1 → 1 |
| Shared dependency | 56.38 s | 60.39 s | 1 → 1 |
| Denied problems | 24.86 s | 39.49 s | 1 → 1 |
| Missing tests and logs | 28.55 s | 28.26 s | 1 → 1 |
| Dependency cycle | 58.63 s | 73.85 s | 1 → 1 |
| Depth boundary | 48.22 s | 51.45 s | 1 → 1 |
| Exact green | 14.11 s | 14.49 s | 1 → 1 |
| Secret in tests | 64.85 s | 77.09 s | 1 → 2 |

The secret task performs two failure reads plus a view across two calls; its
extra discovery/read cost is retained. Wrapper native launches increase 80 → 92
and authenticated GETs 71 → 79 against the earlier optimized sample. The fresh
native control uses 107 launches and 116 GETs. Output-token parity remains unmet:
3011 versus 1292 median, and every wrapper task emits more tokens. Exact-green
timing also remains slightly higher than native, 14.49 versus 14.37 seconds.
Output tokens count stdout/stderr only, excluding prompts, framing and reasoning.

See the unmodified [complete raw report](../evaluations/results/v0.1.1/agent.raw.json),
[independent grades](../evaluations/results/v0.1.1/agent.grades.json) and
[enriched comparison](../evaluations/results/v0.1.1/agent.json). These bind the
frozen evaluated binary and prepared archive; all 33 tool calls, 199 native
launches, 195 authenticated GETs and 283 event notifications were reviewed.
The earlier partial attempt below is disclosed separately and is not pooled
into this complete comparison.

## Verification and independent review

All 171 deterministic top-level tests pass under race on Go 1.27.1 and CI's
Go 1.26.0. Formatting, compilation, vet, generated command drift and Go-only
source checks pass. The checksum-pinned official CLI suite passes 56 additional
contract scenarios; all 18 restricted live tests pass on the original TeamCity
2026.2 build 238924 fixture. The live permission assertion retains exactly the
project-view reader's inventory and foreign-project denial; no write permission
is added. Missing fixtures/binaries fail rather than skip.

Independent source review accepts sanitizer pass order, UTF-8/control handling,
equivalent root/meta protection, both validation gates, stable fingerprints,
exact retrieval scopes and generic skill guidance. The retained
[case generator](../evaluations/results/v0.1.1/sanitizer-differential.go.txt) and
[11,253 differential records](../evaluations/results/v0.1.1/sanitizer-differential.jsonl)
match the previous output.go byte-for-byte. All 28 captured JSON/TOON documents
also match the old renderer, including budget fallback and compacted diagnostics.
Additional tests cover rune boundaries, malformed UTF-8, unsupported/cyclic/NaN
input and a valid envelope rejected by its command payload schema. Input
immutability, optional-change reduction and concurrent fail-closed checks pass.

Review caught a test input that failed the envelope before reaching the payload
gate; it now asserts the payload-specific failure. Initial capture setup lacked
the destination directory; it was created before accepting any baseline. These
were resolved test/setup issues, not discarded model or performance rows.
LSP cannot start because gopls is absent; Go compile/vet/race checks supply the
fallback. No CPU profile was needed to demonstrate these redundant-work savings.

The certified archive contains seven allowed files and the exact evaluated
executable. All 56 offline help/schema commands pass with PATH=/nonexistent and
no configuration, credentials or language runtime. Darwin amd64/arm64 and Linux
arm64 cross-builds pass; execution remains certified only on Linux amd64. See
[the verification record](release-v0.1.1-verification.json) for source, binary,
archive, review, report and test-inventory hashes.
The separate [final release review](../evaluations/results/v0.1.1/release-review.json)
checks the published claims and artifact bindings before tagging.

## Evaluation interruption and retry

The first actual-model attempt stopped at native depth-boundary, after ten
completed sessions. Each completed answer and full trace passes independent
grading; the attempt remains partial and does not satisfy the complete release
gate. The old evaluator returned `Model turn failed: <nil>` before recording the
failed turn, so that turn's trace and costs cannot be reconstructed and its
underlying runtime/provider cause is unknown. The
[raw partial attempt](../evaluations/results/v0.1.1/agent-attempt-1.raw.json),
[partial grading](../evaluations/results/v0.1.1/agent-attempt-1.grades.json) and
[failure log](../evaluations/results/v0.1.1/agent-attempt-1.log) are retained.

The harness-only correction records failed sessions before returning an error,
keeps partial outputs/costs from launched tools and excludes unstarted processes.
It normalizes unexpected failure status values and withholds provider error
prose. Three new regressions check private failure metadata, durable failed-row
persistence and launched-versus-unstarted evidence. Completed-session prompts,
metrics, model settings, corpus, safety bounds and product executable are
unchanged. A fresh complete paired run is used for certification; partial
observations are not pooled into its medians. All attempts remain disclosed.
