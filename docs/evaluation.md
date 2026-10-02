# Measured evaluation

The development release now has an executable, sanitized
[eight-task corpus and protocol](https://github.com/Periecle/teamcity-axi/blob/main/evaluations/README.md). The full
[recorded report](https://github.com/Periecle/teamcity-axi/blob/main/evaluations/results/linux-x64-node24-native1.5.0.json) includes
per-task outputs and regressions. This comparison measures scripted evidence
workflows, not model-agent accuracy or agent-facing tool turns.

Recorded on Linux x64, Node 24.14.0, native TeamCity CLI 1.5.0, with three paired
repetitions per task and a fresh projecting fixture server. Tokens use the pinned
`js-tiktoken@1.0.21` / `o 200 k_base` tokenizer (GPT-4 o vocabulary), counting original
stdout/stderr separately. The benchmark records actual commands, subprocesses,
HTTP requests, and latency; public traces contain only synthetic identities and
scrubbed canaries/paths.

| Workflow                   | Evidence checks passed | Median output tokens | Median wall time | Median native processes | Calls exposing canary |
| -------------------------- | ---------------------- | -------------------: | ---------------: | ----------------------: | --------------------: |
| wrapper-toon               | 24/24                  |                 3023 |         303.4 ms |                     6.5 |                     0 |
| wrapper-json               | 24/24                  |                 2717 |         305.1 ms |                     6.5 |                     0 |
| native-selected-json       | 21/24                  |               1007.5 |          69.0 ms |                     5.5 |                     6 |
| native-failure-diagnostics | 0/24                   |                  238 |          17.6 ms |                       1 |                     3 |

Version startup added a median 6.2 ms over bare Node in nine paired samples; the probes made zero HTTP requests.

The wrapper retained every required task fact, remained within its configured
24 KiB response limit, and exposed no secret canary. Native selected-field JSON
retained all required facts on the seven non-secret tasks; its raw text exposed
the synthetic credential in the secret task. Standalone native failure
diagnostics omit graph, boundary, and source-availability evidence needed by
these compound tasks. Their small output is not a complete equivalent of the
selected-field baseline, and missing metadata is not silently scored as zero.

The selected-field native baseline is smaller and faster than the wrapper in
this corpus. There is no demonstrated token or latency saving. The wrapper's
measured value here is consolidated typed evidence with explicit scope, source
availability, retrieval references, and redaction. The actual model comparison below records agent-facing turns separately. Scripted
report fields stay null because they do not measure model behavior.

These results do not establish live server support, other platform support, or
full release certification. Existing focused live contracts and the actual
compatibility matrix remain separate. Deterministic/native tests enforce the
corpus's evidence and security checks on future changes.

## Actual model-agent evaluation

The [recorded 16-session report](../evaluations/results/linux-x64-node24-gpt6.1-sol-agent.json)
contains all prompts, tool requests/results, final answers, original CLI-text
metrics and independent grades. Each condition used Codex CLI 0.160.0,
`gpt-6.1-sol`, `xhigh`, the same task scope and frozen synthetic evidence, and a
fresh thread/config/fixture server. Condition order alternated by task. Native
agents received verified selected-field JSON, bounded-tail and failure-diagnostic
operations; both conditions could batch or parallelize commands. The oracle,
fixture implementation and real model auth were absent from the tool filesystem.
Trace review found no oracle access, unexpected tool or process-count bypass.

| Metric                                         | Wrapper | Native |
| ---------------------------------------------- | ------: | -----: |
| Independently graded successes                 |     8/8 |    8/8 |
| Identity/completeness/unsupported-cause errors |       0 |      0 |
| Tool/final-answer secret exposures             |       0 |      0 |
| Median agent-facing tool calls                 |       3 |    3.5 |
| Total agent-facing tool calls                  |      30 |     26 |
| Median original stdout/stderr tokens           |    4655 | 1603.5 |
| Median session elapsed time                    |  37.3 s | 85.2 s |
| Total native subprocesses                      |     106 |     95 |
| Total HTTP requests                            |      88 |     99 |

| Task                   | Wrapper calls | Native calls |
| ---------------------- | ------------: | -----------: |
| root-failure           |             2 |            3 |
| shared-dependency      |             5 |            4 |
| denied-problems        |             3 |            3 |
| missing-tests-and-logs |             3 |            3 |
| dependency-cycle       |             3 |            4 |
| depth-boundary         |             3 |            4 |
| exact-green            |             3 |            1 |
| secret-in-tests        |             8 |            4 |

The wrapper has a lower median in this sample but more total calls. Shared
Dependency, exact green and secret-task calls regress. The secret session
included a failed host-CWD command, help/schema exploration and evidence
expansion/recovery; all costs remain counted. Its selected problem-detail
expansion returned NOT_FOUND in this synthetic fixture, while the initial page
remained valid. Both agents preserved that distinction and withheld secrets.
No token saving or broad superiority claim follows from these results.

This is one paired observation per task on fixed synthetic evidence, not a
statistical performance guarantee or additional live compatibility evidence.
CLI token counts exclude RPC framing, prompts and reasoning; provider usage
records remain in the trace. The provider reports the model alias rather than
an immutable backend weight revision. The evaluated production candidate is
`8c0e727` / 0.1.0-dev.1; the final 0.1.0 changes its banner, documentation, license
and package metadata, with no command-behavior change. Package/runtime/native/
corpus and executed-harness hashes, plus the executed source snapshot are
retained for attribution. Current runner cleanup and isolated-CWD guidance were
hardened after the successful run; use the maintained script for new runs.
