# Reproducible failure investigation evaluation

This corpus exercises exact-run failure investigation, duplicate and muted test
identities, a shared dependency, a cycle, depth boundaries, denied sources,
missing logs, green execution, and a synthetic credential canary. It uses the
released native CLI against a projecting loopback fixture server. Both conditions
see the same fixed evidence and task scope. These are synthetic contracts, not
live TeamCity acceptance or model-agent task results.

Run on Node 24 with the checksum-pinned native executable:

```sh
npm ci --ignore-scripts
TEAMCITY_AXI_TEST_BINARY=/absolute/path/to/teamcity npm run evaluate
```

The default is three repetitions; `TEAMCITY_AXI_EVAL_REPETITIONS` accepts 1–10.
`TEAMCITY_AXI_EVAL_OUTPUT` selects the report path, which defaults to ignored
`test-results/evaluation.json`. Each report includes the corpus hash, native
checksum, runtime/platform, paired startup samples, every command/output/exit,
and per-task scores. CI runs one repetition in the native contract suite.
The [recorded report](results/linux-x64-node24-native1.5.0.json) and
[measured comparison](../docs/evaluation.md) retain the results rather than only
a favorable aggregate.

## Compared workflows

- **Wrapper TOON and JSON:** one actual `run failure` invocation using the task's
  declared depth and diagnosis bound. The shared-DAG task uses the default three
  diagnosed executions; its shared leaf remains visible but unexamined. Public
  schemas and successful process exits are checked before evidence scoring.
- **Native selected-field JSON:** exact detail and minimal projected REST pages,
  immediate dependency counts and traversal, distinct unmuted/muted test reads,
  problems, and contextual changes. Independent requests overlap in batches of
  three. A bounded log tail is requested only when direct evidence is missing.
  There is no full-log download, redundant summary call, or inflated default
  field projection. The server applies selected fields before capture.
- **Native failure diagnostics:** the actual optimized `run log --failed --json`
  command is also measured separately. Its standalone summary is useful but
  lacks the graph/source-availability information needed for this compound
  rubric. It is not padded with unrelated commands or treated as a complete
  equivalent of the selected-field workflow.

Native requests and fixture positions are fixed by this corpus. The evaluator
creates its own server and isolated configuration; it does not accept a live
server URL or import real credentials. Do not use the scripted traversal as a
general TeamCity client: arbitrary retained pages would require more protocol
work than these small fixtures contain.

## Measurement and scoring

Output tokens are counted from each original stdout and stderr channel with
`js-tiktoken@1.0.21`, `o200k_base` (GPT-4o vocabulary). Special-token spellings in
text are encoded as ordinary text. Counts exclude prompts, reasoning, and chat
framing. The model vocabulary names the tokenizer; no GPT-4o inference is run.
Tokenization happens after timed command execution. A random synthetic canary
is checked before scrubbing public traces; paths, loopback ports, and canary text
are replaced afterward. Counts describe the original channels, so re-tokenizing
the scrubbed artifact is not expected to reproduce canary-containing counts.

Latency covers each scripted workflow, including child startup and orchestration.
Conditions alternate order between repetitions. Native subprocess counts and
HTTP request counts are separate. Wrapper counts include its native version
probe. Nine alternating bare-Node/`--version` pairs measure startup; version
probes must make zero HTTP requests. Host timings are observations, not portable
performance guarantees.

The rubric verifies required retained identities, finished lifecycle/result,
graph edges/cycles/boundaries, compound occurrence attribution, and required
source availability. It rejects foreign nodes/jobs/projects and references,
denied-as-empty sources, unsupported complete-coverage claims, and canary
exposure in the canary task. Identity/completeness counters cover these explicit
contradictions; they do not measure a model's reasoning. Native diagnostics do
not provide all identity or source metadata, which remains unverified rather
than counted as an invented identity error or a verified zero.

`agentTaskSuccess`, `agentFacingToolTurns`, and `unjustifiedCausalClaims` are
explicitly null. An orchestrator's process launch count is not an agent-facing
turn, and retaining evidence is not proof that an agent answers correctly.

## Model-agent release gate

Use the fixed prompts in fresh, isolated agent sessions. Hide the oracle and
fixture implementation from the agent. Counterbalance native and wrapper
conditions with the same model/version/settings, task scope, read identity, and
evidence. Give native agents its documented selected-field JSON and failure
diagnostics; allow batching and parallel shell commands in both conditions.
Use the portable wrapper skill for the wrapper condition. Record the exact
model identity and all tool requests/results, final answers, and wall time.

Count each actual agent-facing tool invocation once, including a shell call
that batches several native processes. Count internally repeated HTTP calls
only in the separate HTTP metric. Independently grade answers against the
oracle for task success, wrong run/job/root identity, completeness mistakes,
unsupported causal claims, and exposed secrets. Report per-task failures as
well as medians. A lower median turn count is acceptable only without lower
task success or any correctness/security regression. The recorded model-agent run and independent grades are linked below; the
scripted report remains a separate measurement.

## Running the model evaluation

`node scripts/evaluate-agent.mjs` uses a local Codex app-server and one actual
client-executed shell tool. Requires Linux `bwrap`, Node 24, the checksum-pinned
native binary, a private local model-auth path, and a production-only runtime.
The runtime contains only `node_modules/teamcity-axi/{bin,dist,schemas,package.json}`
and its separately installed production dependencies. Exclude project docs,
tests, scripts and the evaluation corpus from that filesystem. The portable skill
is provided in the wrapper prompt; documented selected-field JSON, bounded tails
and failure diagnostics are provided in the native prompt.

Set `TEAMCITY_AXI_AGENT_RUNTIME`, `TEAMCITY_AXI_AGENT_AUTH_PATH`,
`TEAMCITY_AXI_TEST_BINARY` and optionally `TEAMCITY_AXI_AGENT_OUTPUT`. Model and
effort defaults are `gpt-6.1-sol` and `xhigh`; override with
`TEAMCITY_AXI_AGENT_MODEL` / `TEAMCITY_AXI_AGENT_EFFORT` only for a separately
recorded comparison. Optional package/checkpoint variables preserve artifact
provenance. This is trusted local release tooling, excluded from the package and
public CI. No model-auth material is placed in the shell filesystem or published.
Temporary client credentials and sessions are deleted on success, setup failure
and protocol timeout.

Each task gets a fresh thread/config/fixture server, with condition order
alternating by task. Both conditions may batch or parallelize commands within one
shell invocation. Unexpected built-in tools invalidate the row. Tool requests,
results, final answers, provider-reported alias/settings, CLI/runtime/artifact
provenance, process/HTTP counts and original stdout/stderr token counts are
recorded. The process logger is auditable instrumentation; independent trace
review rejects bypass or metric tampering. There is no claim that it is
tamper-proof. CLI text counts exclude RPC JSON framing, prompts and reasoning;
provider usage events remain in the trace separately.

Answers must be independently graded before accepting the report. One paired
sample per task is an exploratory fixed-corpus observation, not a statistical
performance guarantee or evidence of support for additional server/platform
combinations. The provider reports a model alias rather than an immutable backend
weight revision. Canaries, loopback ports and temporary paths are scrubbed only
after original output metrics are computed.

The [actual model-agent report](results/linux-x64-node24-gpt6.1-sol-agent.json)
records16 independently graded sessions; both conditions passed all8 tasks.
See [the measured comparison](../docs/evaluation.md) for every task, total calls
and per-task regressions. The executed source snapshot is an audit record; use
the maintained runner, which includes stronger failure cleanup, for new runs.
