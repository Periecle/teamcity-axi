# Measured evaluation

The development release now has an executable, sanitized
[eight-task corpus and protocol](https://github.com/Periecle/teamcity-axi/blob/main/evaluations/README.md). The full
[recorded report](https://github.com/Periecle/teamcity-axi/blob/main/evaluations/results/linux-x64-node24-native1.5.0.json) includes
per-task outputs and regressions. This comparison measures scripted evidence
workflows, not model-agent accuracy or agent-facing tool turns.

Recorded on Linux x64, Node 24.14.0, native TeamCity CLI 1.5.0, with three paired
repetitions per task and a fresh projecting fixture server. Tokens use the pinned
`js-tiktoken@1.0.21` / `o200k_base` tokenizer (GPT-4o vocabulary), counting original
stdout/stderr separately. The benchmark records actual commands, subprocesses,
HTTP requests, and latency; public traces contain only synthetic identities and
scrubbed canaries/paths.

| Workflow                   | Evidence checks passed | Median output tokens | Median wall time | Median native processes | Calls exposing canary |
| -------------------------- | ---------------------- | -------------------: | ---------------: | ----------------------: | --------------------: |
| wrapper-toon               | 24/24                  |                 2994 |         306.4 ms |                     6.5 |                     0 |
| wrapper-json               | 24/24                  |                 2694 |         312.7 ms |                     6.5 |                     0 |
| native-selected-json       | 21/24                  |               1007.5 |          67.5 ms |                     5.5 |                     6 |
| native-failure-diagnostics | 0/24                   |                  238 |          17.7 ms |                       1 |                     3 |

Version startup added a median 4.9 ms over bare Node in nine paired samples; the probes made zero HTTP requests.

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
availability, retrieval references, and redaction. A claim of fewer agent-facing
turns still requires the isolated model-agent evaluation described in the
protocol, with native batching allowed. Task success, turn count, and causal
claim metrics remain null until that evaluation runs.

These results do not establish live server support, other platform support, or
full release certification. Existing focused live contracts and the actual
compatibility matrix remain separate. Deterministic/native tests enforce the
corpus's evidence and security checks on future changes.
