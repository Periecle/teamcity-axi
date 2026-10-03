# Measured Go and TypeScript evaluation

The Go release has separate scripted and actual model-agent evaluations. Both
use the same eight-task corpus and checksum-pinned official TeamCity CLI 1.5.0.
Synthetic observations establish this corpus's behavior; they do not certify
additional live servers or platforms.

## Scripted evidence comparison

The [release-binary Go report](../evaluations/results/linux-x64-go1.27-native1.5.0-release.json)
records 96 observations: eight tasks, four conditions and three repetitions.
It binds the corpus, native binary and exact packaged Go executable hashes.
The [historical TypeScript report](../evaluations/results/linux-x64-node24-native1.5.0.json)
records the equivalent corpus on Node 24.14.0. Go ran on Linux amd64 with Go 1.27.1.

| Workflow | Evidence/safety score, both versions | Tokens TS → Go | Median time TS → Go | Go native processes | Go canary-exposing calls |
| --- | --- | ---: | ---: | ---: | ---: |
| wrapper-toon | 24/24 | 3023 → 3025 | 303.4 → 111.5 ms | 6.5 | 0 |
| wrapper-json | 24/24 | 2717 → 2718 | 305.1 → 108.4 ms | 6.5 | 0 |
| native-selected-json | 21/24 | 1007.5 → 1009.5 | 69.0 → 46.2 ms | 5.5 | 6 |
| native-failure-diagnostics | 0/24 | 238 → 238 | 17.6 → 13.5 ms | 1 | 3 |

Both Go formats retain 24/24 required observations with zero identity,
completeness or canary errors. The optimized selected-field native workflow
retains the seven non-secret tasks but exposes the synthetic canary in the
secret task. Standalone native failure diagnostics omit graph, traversal-boundary
and independent-source facts needed by this compound rubric; its 0/24 score does
not mean that the native diagnostic command is useless or broken.

Go wrapper latency is about 63–64% lower than the historical TypeScript run,
with nearly identical output token counts. Native timing also improved between
runs, so these are host observations rather than a controlled language-effect
estimate. The direct native workflows remain faster and smaller. JSON is smaller
than TOON in this corpus. Neither general token savings nor general native-latency
superiority is claimed. Version startup medians are 29.3 ms for TypeScript and
5.8 ms for Go; both make zero HTTP requests, and their no-op baselines differ.

The historical Go handoff benchmark had cumulative HTTP counts across scripted
workflows. It remains unchanged for audit and must not be used for per-workflow
HTTP cost comparisons. Independent review found the defect; the release runner
now resets the request log per condition and verifies every task's exact count.
The corrected release report has median HTTP counts 5.5 for both wrapper formats
and selected-field native JSON, and four for native failure diagnostics. The
actual model sessions use fresh fixture servers and were unaffected.

## Actual model-agent comparison

The [Go model report](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-agent.json)
contains 16 actual sessions, backed by its
[immutable raw report](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-agent.raw.json)
and [independent answer/trace grading](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-agent.grades.json).
The [historical TypeScript model report](../evaluations/results/linux-x64-node24-gpt6.1-sol-agent.json)
is a separate 16-session observation. Both requested gpt-6.1-sol, xhigh effort,
Codex CLI 0.160.0 and one paired sample per task. Conditions alternate order and
use fresh isolated threads/fixtures; native agents may batch projected REST and
bounded diagnostic reads, while wrapper agents receive the portable skill.

| Metric | TS AXI | TS CLI | Go AXI | Go CLI |
| --- | ---: | ---: | ---: | ---: |
| Task success | 8/8 | 8/8 | 8/8 | 8/8 |
| Median tool calls | 3 | 3.5 | 3 | 3 |
| Total tool calls | 30 | 26 | 30 | 22 |
| Median output tokens | 4655 | 1603.5 | 5576.5 | 861.5 |
| Median wall time | 37.3 s | 85.2 s | 55.4 s | 105.1 s |
| Median native processes | 12 | 8 | 14.5 | 7 |
| Median HTTP requests | 10.5 | 10.5 | 12 | 8 |
| Identity/completeness/causal errors | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 |
| Tool/final canary exposures | 0/0 | 0/0 | 0/0 | 0/0 |

Both versions and their native conditions pass 8/8 tasks without correctness or
security errors. Go AXI's median tool calls tie native at three: the lower-median
performance target is **not met**. Its total calls are higher, 30 versus 22, and
its median output is about 6.5 times larger. Its recorded median task time is
lower, 55.4 versus 105.1 seconds, but the TypeScript AXI sample was faster and
smaller than the Go sample. The two historical comparisons do not isolate a Go
language effect: model behavior, prompts/harnesses and host conditions can vary,
and the provider reports an alias without an immutable backend weight revision.

Per-task tool calls expose both benefits and regressions:

| Task | TS AXI | TS CLI | Go AXI | Go CLI |
| --- | ---: | ---: | ---: | ---: |
| root-failure | 2 | 3 | 5 | 2 |
| shared-dependency | 5 | 4 | 2 | 4 |
| denied-problems | 3 | 3 | 3 | 2 |
| missing-tests-and-logs | 3 | 3 | 3 | 3 |
| dependency-cycle | 3 | 4 | 2 | 4 |
| depth-boundary | 3 | 4 | 3 | 3 |
| exact-green | 3 | 1 | 3 | 1 |
| secret-in-tests | 8 | 4 | 9 | 3 |

Shared-dependency and cycle investigations each use two wrapper calls versus
four native calls while retaining task-critical evidence. Root failure, denied
problems, green execution and the secret task regress. The secret wrapper trace
includes invalid schema requests, repeated investigations and exact-detail/page
expansion; all nine calls and failed reads remain counted. No general fewer-call,
token-saving or performance guarantee follows from this sample. The recorded
per-task call benefits satisfy the narrow multi-source demonstration; they do
not certify the unmet lower-median target.

Independent review reconciled all 52 tool results, 221 native launches and
208 authenticated GETs. No credential inspection, instrumentation bypass,
mutation or unsupported remote access was observed. Scripted raw native canary
exposures and model-tool canary results are different measurements: model agents
withheld credential-bearing detail, giving zero exposures in both conditions.

## Provenance and limits

The [interrupted first attempt](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-agent.aborted.json)
contains one completed wrapper session; its following native session was
interrupted when review found missing aggregate protocol capture bounds. The
runner received a 64 MiB full-stream ceiling and overflow/cleanup regression,
then all 16 certified sessions restarted. The first attempt is excluded from
certified metrics and preserved for audit. It is not silently substituted.

Original stdout and stderr are tokenized separately before portable trace
scrubbing, using pinned ordinary-text o200k_base tokenizers. Prompts, model
reasoning and RPC framing are excluded; provider usage events remain in the
trace separately. Retokenizing scrubbed canaries is not expected to reproduce
original-channel counts. Both historical TypeScript artifacts and the original
Go handoff benchmark remain unchanged. See the
[evaluation protocol](../evaluations/README.md) and
[release verification](release-verification.json) for reproduction and artifact
bindings. Live contracts, package checks and platform support remain separate.
