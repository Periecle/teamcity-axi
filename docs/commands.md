# Command reference

Generated for teamcity-axi 0.1.1 from the executable command registry.
Run `make docs` after changing commands; `make test` rejects drift.

Use a registered server alias. Example IDs are placeholders, not discovered resources.
These commands only read TeamCity. Status checks the local committed checkout;
watch checks the terminal outcome of one fixed execution. A normal red observation
exits zero; `--check` fails its assertion with exit one.

## status

CI orientation for the exact current checkout.

```sh
teamcity-axi status --job=Payments_Build --server=work
```

```text
teamcity-axi status
CI orientation for the exact current checkout

  --all-branches  Include all logical branches
  --branch VALUE  Logical branch or @this
  --check  Assert completed successful exact checkout
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --literal-branch VALUE  Literal logical branch
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --revision VALUE  Commit identity or @head
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --vcs-root VALUE  Exact VCS root ID
  --version  Show application version
```

## context.show

Resolve local trusted context.

```sh
teamcity-axi context show
```

```text
teamcity-axi context show
Resolve local trusted context

  --all-branches  Include all logical branches
  --branch VALUE  Logical branch or @this
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --literal-branch VALUE  Literal logical branch
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --revision VALUE  Commit identity or @head
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --vcs-root VALUE  Exact VCS root ID
  --verify  Verify selected scope remotely
  --version  Show application version
```

## doctor

Inspect dependency, authentication and capabilities.

```sh
teamcity-axi doctor --server=work
```

```text
teamcity-axi doctor
Inspect dependency, authentication and capabilities

  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --offline  Never perform network reads
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## schema

Inspect a packaged command contract.

```sh
teamcity-axi schema run.view
```

```text
teamcity-axi schema <command>
Inspect a packaged command contract

  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.list

Read one bounded scoped execution page.

```sh
teamcity-axi run list --job=Payments_Build --server=work
```

```text
teamcity-axi run list
Read one bounded scoped execution page

  --all-branches  Include all logical branches
  --branch VALUE  Logical branch or @this
  --cursor VALUE  Opaque bounded continuation
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --fields VALUE  Comma separated public fields
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --limit VALUE  Maximum requested rows
  --literal-branch VALUE  Literal logical branch
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --result VALUE  Outcome filter
  --revision VALUE  Commit identity or @head
  --server VALUE  Registered trusted server alias
  --since VALUE  RFC 3339 finish-time lower bound
  --state VALUE  Lifecycle filter
  --timeout VALUE  Overall deadline (ms, s or m)
  --until VALUE  RFC 3339 finish-time upper bound
  --vcs-root VALUE  Exact VCS root ID
  --version  Show application version
```

## run.view

Observe one exact execution.

```sh
teamcity-axi run view 482193 --server=work
```

```text
teamcity-axi run view <runId>
Observe one exact execution

  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --fields VALUE  Comma separated public fields
  --format VALUE  Output serialization
  --full  Expand text previews within budgets
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.problems

Read independent problem occurrences.

```sh
teamcity-axi run problems 482193 --server=work
```

```text
teamcity-axi run problems <runId>
Read independent problem occurrences

  --cursor VALUE  Opaque bounded continuation
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --full  Expand text previews within budgets
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --limit VALUE  Maximum requested rows
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --problem VALUE  Exact problem occurrence ID
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.tests

Read independent test occurrences.

```sh
teamcity-axi run tests 482193 --server=work
```

```text
teamcity-axi run tests <runId>
Read independent test occurrences

  --cursor VALUE  Opaque bounded continuation
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --failed  Unmuted failures
  --fields VALUE  Comma separated public fields
  --format VALUE  Output serialization
  --full  Expand text previews within budgets
  --help  Show local command help
  --include-muted  Include muted failures with --failed
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --limit VALUE  Maximum requested rows
  --max-bytes VALUE  Serialized stdout byte ceiling
  --muted  Muted failures
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --test VALUE  Exact test occurrence ID
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.log

Read a bounded structured log tail.

```sh
teamcity-axi run log 482193 --server=work
```

```text
teamcity-axi run log <runId>
Read a bounded structured log tail

  --contains VALUE  Literal filter within the fetched window
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --failed  Failure-oriented evidence
  --format VALUE  Output serialization
  --full  Expand text previews within budgets
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --tail VALUE  Tail messages
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.changes

Read bounded contextual changes.

```sh
teamcity-axi run changes 482193 --server=work
```

```text
teamcity-axi run changes <runId>
Read bounded contextual changes

  --cursor VALUE  Opaque bounded continuation
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --fields VALUE  Comma separated public fields
  --files  Include bounded changed file names
  --format VALUE  Output serialization
  --full  Expand text previews within budgets
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --limit VALUE  Maximum requested rows
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.tree

Traverse bounded snapshot execution graph.

```sh
teamcity-axi run tree 482193 --server=work
```

```text
teamcity-axi run tree <runId>
Traverse bounded snapshot execution graph

  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --depth VALUE  Maximum snapshot traversal depth
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --max-nodes VALUE  Maximum unique executions
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.failure

Investigate independent failure evidence.

```sh
teamcity-axi run failure 482193 --server=work
```

```text
teamcity-axi run failure <runId>
Investigate independent failure evidence

  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --depth VALUE  Maximum snapshot traversal depth
  --format VALUE  Output serialization
  --full  Expand text previews within budgets
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --max-diagnosed-runs VALUE  Maximum diagnosed executions including root
  --max-nodes VALUE  Maximum unique executions
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## run.watch

Observe one execution until terminal or deadline.

```sh
teamcity-axi run watch 482193 --server=work
```

```text
teamcity-axi run watch <runId>
Observe one execution until terminal or deadline

  --check  Assert terminal success
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --interval VALUE  Poll interval (ms, s or m)
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## job.list

Read scoped jobs.

```sh
teamcity-axi job list --project=Payments --server=work
```

```text
teamcity-axi job list
Read scoped jobs

  --cursor VALUE  Opaque bounded continuation
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --json  Alias for --format json
  --limit VALUE  Maximum requested rows
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## job.view

Read safe exact job metadata.

```sh
teamcity-axi job view Payments_Build --server=work
```

```text
teamcity-axi job view <id>
Read safe exact job metadata

  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## queue.list

Read scoped queued executions.

```sh
teamcity-axi queue list --job=Payments_Build --server=work
```

```text
teamcity-axi queue list
Read scoped queued executions

  --cursor VALUE  Opaque bounded continuation
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --limit VALUE  Maximum requested rows
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## agent.list

Read scoped agent availability.

```sh
teamcity-axi agent list --job=Payments_Build --server=work
```

```text
teamcity-axi agent list
Read scoped agent availability

  --cursor VALUE  Opaque bounded continuation
  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --limit VALUE  Maximum requested rows
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --pool VALUE  Exact agent pool ID
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```

## agent.view

Read safe exact agent metadata.

```sh
teamcity-axi agent view 7 --server=work
```

```text
teamcity-axi agent view <id>
Read safe exact agent metadata

  --cwd VALUE  Resolve worktree context from this directory
  --debug  Redacted metadata on stderr
  --format VALUE  Output serialization
  --help  Show local command help
  --job VALUE  Exact job ID
  --json  Alias for --format json
  --max-bytes VALUE  Serialized stdout byte ceiling
  --no-hints  Omit optional read suggestions
  --pool VALUE  Assert exact agent pool ID
  --project VALUE  Exact project ID
  --require-complete  Fail if the requested answer is partial
  --server VALUE  Registered trusted server alias
  --timeout VALUE  Overall deadline (ms, s or m)
  --version  Show application version
```
