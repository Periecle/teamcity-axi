# Measured Go evaluation

The [Go report](../evaluations/results/linux-x64-go1.27-native1.5.0.json) records
96 scripted observations: eight tasks, four conditions and three repetitions.
It ran on Linux amd64, Go 1.27.1, using checksum-pinned official TeamCity CLI
1.5.0 against a projecting local HTTP fixture. The report binds the corpus,
native binary and executed Go wrapper hashes. It does not measure model-agent
accuracy or agent-facing tool turns.

| Workflow | Required evidence retained | Median output tokens | Median wall time | Median native processes | Calls exposing synthetic canary |
| --- | --- | ---: | ---: | ---: | ---: |
| wrapper-toon | 24/24 | 3025 | 111.4 ms | 6.5 | 0 |
| wrapper-json | 24/24 | 2718 | 109.9 ms | 6.5 | 0 |
| native-selected-json | 21/24 | 1009.5 | 47.0 ms | 5.5 | 6 |
| native-failure-diagnostics | 0/24 | 238 | 13.5 ms | 1 | 3 |

Both wrapper formats retained all required evidence, respected output budgets,
and had zero identity/completeness mistakes or synthetic credential exposures.
The native selected-field workflow retained the seven non-secret tasks; direct
native text exposed the canary in the secret task. Standalone native failure
diagnostics omitted graph, boundary and source-availability facts required by
this compound rubric. Those omissions remain visible in every task row.

TOON is the primary output format. JSON was smaller in this corpus; neither a
token saving nor a general latency advantage over native workflows is claimed.
Counts use pinned Go o200k_base tokenization, verified against the original
js-tiktoken ordinary-text counts. Original stdout and stderr are counted
separately before portable trace scrubbing. Prompt/reasoning/RPC framing is
excluded. Version probes made zero HTTP requests. Timings are host observations.

The model-evaluation runner is also implemented in Go. Its deterministic tests
exercise protocol requests/results, tool restrictions, token accounting,
filesystem selection, secret scrubbing, process/capture limits and timeout
cleanup. An auth-free Linux bubblewrap probe passed. New credential-backed model
sessions were not launched: automatic approval review requires explicit user
authorization to use model credentials and account usage. The requested approval
is pending. Therefore the Go model release gate remains unexecuted, and no new
model success or tool-turn claim is made.

The [historical scripted report](../evaluations/results/linux-x64-node24-native1.5.0.json)
and [historical 16-session model report](../evaluations/results/linux-x64-node24-gpt6.1-sol-agent.json)
remain immutable audit records of the former TypeScript implementation. They do
not certify Go. Reproduce fresh observations with the maintained Go commands in
[the evaluation protocol](../evaluations/README.md); independently grade model
answers and tool traces before accepting a new model report.

Synthetic evaluations do not certify additional live TeamCity versions or
platforms. Live contracts and package acceptance remain separate evidence.
