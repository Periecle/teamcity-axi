# Post-release Go performance

The slow historical number measured a complete model session, including model
reasoning and tool round trips. The historical Go trace spent less wall time
executing tools than the TypeScript trace. In the published Go model sample, all
wrapper tool execution together took 2.05 seconds out of 594.26 seconds of session time
(0.345%). Median cumulative tool execution per task was 212 ms, versus 576 ms
in the historical TypeScript sample. These values are computed from the retained
call durations in the [Go raw report](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-agent.raw.json)
and [TypeScript report](../evaluations/results/linux-x64-node24-gpt6.1-sol-agent.json).

The Go model trace spent extra turns discovering help/schema syntax, repeating
investigations and expanding detail already retained. The secret task used nine
calls. These observations explain where to focus product changes; they do not
establish a language or model-provider cause for the historical session timings.

## Changes and safety

The optimized product checkpoint is `5300c8acc4aeb9db85bbe85670ac83f9dffaa2eb`.
It compiles only the offline schemas actually needed by a command and serializes
compiler/cache access. Every emitted envelope and payload is still validated.
Failure reports promote a detail hint only when a retained evidence excerpt was
actually shortened. All evidence keeps its exact scoped retrieval command;
summary-only truncation does not imply that supporting detail is missing.
The portable skill supplies generic first-read syntax and explains retained
item/source identities, coverage and optional expansion.

Required evidence acquisition, graph bounds, exact identity, redaction,
unknown counts and partial states are unchanged. The independent security and
evidence reviewer accepted the source changes and their regressions. Corpus task
prompts, fixture evidence, model effort, safety bounds and grading thresholds
remain unchanged. Full session prompts change through the updated portable skill;
it contains generic tool guidance, without corpus identities or oracle answers.

The published v0.1.0 tag, archive and original reports remain unchanged. These
improvements are available from the current source checkout; they are not part
of the existing release download.

## Executable measurements

Thirty alternating cold starts per command and binary are retained in the
[startup report](../evaluations/results/linux-x64-go1.27-performance-startup.json).

| Command | Released Go | Optimized Go |
| --- | ---: | ---: |
| `--version` | 6.11 ms | 6.21 ms |
| `schema run.failure` | 26.22 ms | 17.30 ms |
| `context show` | 24.39 ms | 16.15 ms |

Schema/context startup drops about 34%; the unchanged version path is a control.
These host wall times include scheduling, and the live suite ran during startup
sampling. They are not a general startup guarantee.

Two complete scripted pairs run in reversed outer order, with three internal
repetitions each. All 384 observations remain in the
[baseline](../evaluations/results/linux-x64-go1.27-native1.5.0-performance-baseline.json),
[candidate](../evaluations/results/linux-x64-go1.27-native1.5.0-performance-candidate.json),
[candidate repeat](../evaluations/results/linux-x64-go1.27-native1.5.0-performance-candidate-repeat.json)
and [baseline repeat](../evaluations/results/linux-x64-go1.27-native1.5.0-performance-baseline-repeat.json)
reports. Each row has actual process/output/HTTP measurements and evidence scores.

| Workflow | Released → optimized median | Output tokens | Retained observations |
| --- | ---: | ---: | ---: |
| Wrapper TOON | 111.30 → 103.06 ms | 3025 → 3004 | 48/48 → 48/48 |
| Wrapper JSON | 113.13 → 102.32 ms | 2718 → 2699 | 48/48 → 48/48 |
| Native selected JSON | 52.23 → 47.14 ms | 1009.5 → 1009.5 | 42/48 → 42/48 |
| Native failure diagnostics | 13.56 → 13.36 ms | 238 → 238 | 0/48 → 0/48 |

Both wrapper formats retain all required evidence with zero identity,
completeness or canary errors. Required HTTP/native process counts are unchanged.
Native workflows retain their existing evidence/safety limitations: selected
JSON exposes the synthetic canary, and standalone failure diagnostics omit facts
required by this compound rubric. Their scores do not describe general native
CLI usefulness. Native timings also change between the outer pairs, so the
wrapper's 7–10% observed improvement does not isolate host load from product cost.
The initial baseline report overlapped required race checks; all samples remain
included rather than selecting the quieter or more favorable pair.

## Actual model evaluation

The [fresh model report](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-performance-agent.json)
retains all 16 sessions, with an [unchanged raw report](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-performance-agent.raw.json)
and [independent answer/trace grading](../evaluations/results/linux-x64-go1.27-gpt6.1-sol-performance-agent.grades.json).
It uses the same eight-task corpus, gpt-6.1-sol at xhigh, Codex CLI 0.160.0,
five-minute deadlines, one sample per task/condition, isolated synthetic fixtures
and the checksum-pinned official native CLI. Conditions alternate by task. All
sessions completed without reruns, answer edits or selectively excluded rows.

| Metric | Historical TS AXI | Released Go AXI | Optimized Go AXI | Fresh native CLI |
| --- | ---: | ---: | ---: | ---: |
| Task success | 8/8 | 8/8 | 8/8 | 8/8 |
| Median tool calls | 3 | 3 | 1 | 3 |
| Total tool calls | 30 | 30 | 8 | 24 |
| Median output tokens | 4655 | 5576.5 | 3001.5 | 1090.5 |
| Median session time | 37.3 s | 55.4 s | 44.3 s | 135.3 s |
| Median native processes | 12 | 14.5 | 7.5 | 9 |
| Median HTTP requests | 10.5 | 12 | 6 | 11 |
| Correctness/security errors | 0 | 0 | 0 | 0 |

The optimized wrapper meets the lower-median-call target in this fixed sample
without losing task success or adding errors. Each wrapper task in this sample
uses one actual shell tool call; a call can batch multiple wrapper commands,
which remain separately counted as native processes and HTTP requests. Total calls drop 73%, median
output drops 46%, and median session time drops 20% from released Go. Wrapper
output remains larger than native in every task; the native-output-token target
is unmet. Session time is 67% lower than this fresh native sample, while the
green-build native read remains faster.

| Task | Released Go AXI calls | Optimized Go AXI calls | Fresh CLI calls | Released → optimized Go time |
| --- | ---: | ---: | ---: | ---: |
| root-failure | 5 | 1 | 2 | 92.3 → 40.4 s |
| shared-dependency | 2 | 1 | 4 | 64.5 → 56.4 s |
| denied-problems | 3 | 1 | 3 | 45.5 → 24.9 s |
| missing-tests-and-logs | 3 | 1 | 2 | 43.3 → 28.5 s |
| dependency-cycle | 2 | 1 | 5 | 77.6 → 58.6 s |
| depth-boundary | 3 | 1 | 4 | 46.3 → 48.2 s |
| exact-green | 3 | 1 | 1 | 19.3 → 14.1 s |
| secret-in-tests | 9 | 1 | 3 | 205.4 → 64.8 s |

The depth task is slower than its released-Go observation. Green takes 14.1 s
with the wrapper versus 11.9 s with fresh native CLI. Both regressions remain
in the aggregates. Independent review accepts all 32 tool calls, 188 native
launches and 189 authenticated GETs, with zero identity, completeness,
unsupported-cause or tool/final secret errors.

The 44.3 s median still exceeds historical TypeScript's 37.3 s by 19%; that
historical wall-time target is not beaten. Model alias/backend, host scheduling
and full skill prompts do not form a controlled language comparison. Median
cumulative wrapper tool execution is now 92 ms; all wrapper tools together use
0.28% of session wall time. The remaining session time includes model startup,
reasoning and protocol round trips; these observations do not separate those
costs or promise latency for another model/server. One pair per task is an
exploratory observation, not a statistical performance guarantee.

## Verification

The optimized code passes `make check`, all 165 deterministic tests under race,
the full checksum-pinned official-binary mock suite and all 18 restricted live
tests on the original TeamCity 2026.2 build 238924 fixture. Restored credentials
preserve the reader's existing project-view role and exact permission inventory;
credentials are private and excluded from reports. Missing fixtures never skip.
LSP could not start because `gopls` is missing; compilation, vet and race tests
supply the fallback. Full race checks also pass on CI's Go 1.26.0 toolchain.
[Remote CI](https://github.com/Periecle/teamcity-axi/actions/runs/37128735144)
passes deterministic/race/archive/native checks at `71abfd1`. Its preceding run
hit the existing flood fixture's five-second deadline under race instrumentation;
the test-only correction allows thirty seconds to decode the full 64 MiB stream
and still requires actual overflow, zero retained ignored events and process
reaping within one second. It does not change the product or evaluation harness.
The [performance verification record](performance-verification.json) binds source,
binary/report hashes, both race inventories, live identity/permissions, CI and
the complete independently graded model comparison. The existing release audit
and publication records retain their original checkpoint.
