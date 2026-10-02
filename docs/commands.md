# Command reference

Generated for teamcity-axi 0.1.0-dev.1 from the executable command registry.
Run `npm run docs:generate` after changing commands; `npm test` rejects drift.

Use a registered server alias. Example IDs are placeholders, not discovered resources.
These commands only read TeamCity. Status checks the local committed checkout;
watch checks the terminal outcome of one fixed execution. A normal red observation
exits zero; `--check` fails its assertion with exit one.

## status

CI orientation for the exact current checkout.

```sh
teamcity-axi status --job=Payments_Build --server=work --json
```

```text
teamcity-axi status
CI orientation for the exact current checkout

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --branch VALUE  Logical branch or @this
  --literal-branch VALUE  Literal logical branch
  --all-branches  Include all logical branches
  --revision VALUE  Commit identity or @head
  --vcs-root VALUE  Exact VCS root ID
  --check  Assert completed successful exact checkout
```

## context.show

Resolve local trusted context.

```sh
teamcity-axi context show --json
```

```text
teamcity-axi context show
Resolve local trusted context

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --branch VALUE  Logical branch or @this
  --literal-branch VALUE  Literal logical branch
  --all-branches  Include all logical branches
  --revision VALUE  Commit identity or @head
  --vcs-root VALUE  Exact VCS root ID
  --verify  Verify selected scope remotely
```

## doctor

Inspect dependency, authentication and capabilities.

```sh
teamcity-axi doctor --server=work --json
```

```text
teamcity-axi doctor
Inspect dependency, authentication and capabilities

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --offline  Never perform network reads
```

## schema

Inspect a packaged command contract.

```sh
teamcity-axi schema run.view --json
```

```text
teamcity-axi schema <command>
Inspect a packaged command contract

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
```

## run.list

Read one bounded scoped execution page.

```sh
teamcity-axi run list --job=Payments_Build --server=work --json
```

```text
teamcity-axi run list
Read one bounded scoped execution page

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --branch VALUE  Logical branch or @this
  --literal-branch VALUE  Literal logical branch
  --all-branches  Include all logical branches
  --revision VALUE  Commit identity or @head
  --vcs-root VALUE  Exact VCS root ID
  --limit VALUE  Maximum requested rows
  --cursor VALUE  Opaque bounded continuation
  --fields VALUE  Comma separated public fields
  --state VALUE  Lifecycle filter
  --result VALUE  Outcome filter
  --since VALUE  RFC 3339 finish-time lower bound
  --until VALUE  RFC 3339 finish-time upper bound
```

## run.view

Observe one exact execution.

```sh
teamcity-axi run view 482193 --server=work --json
```

```text
teamcity-axi run view <runId>
Observe one exact execution

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --fields VALUE  Comma separated public fields
  --full  Expand text previews within budgets
```

## run.problems

Read independent problem occurrences.

```sh
teamcity-axi run problems 482193 --server=work --json
```

```text
teamcity-axi run problems <runId>
Read independent problem occurrences

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --limit VALUE  Maximum requested rows
  --cursor VALUE  Opaque bounded continuation
  --problem VALUE  Exact problem occurrence ID
  --full  Expand text previews within budgets
```

## run.tests

Read independent test occurrences.

```sh
teamcity-axi run tests 482193 --server=work --json
```

```text
teamcity-axi run tests <runId>
Read independent test occurrences

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --limit VALUE  Maximum requested rows
  --cursor VALUE  Opaque bounded continuation
  --fields VALUE  Comma separated public fields
  --full  Expand text previews within budgets
  --failed  Unmuted failures
  --muted  Muted failures
  --include-muted  Include muted failures with --failed
  --test VALUE  Exact test occurrence ID
```

## run.log

Read a bounded structured log tail.

```sh
teamcity-axi run log 482193 --server=work --json
```

```text
teamcity-axi run log <runId>
Read a bounded structured log tail

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --tail VALUE  Tail messages
  --contains VALUE  Literal filter within the fetched window
  --failed  Failure-oriented evidence
  --full  Expand text previews within budgets
```

## run.changes

Read bounded contextual changes.

```sh
teamcity-axi run changes 482193 --server=work --json
```

```text
teamcity-axi run changes <runId>
Read bounded contextual changes

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --limit VALUE  Maximum requested rows
  --cursor VALUE  Opaque bounded continuation
  --fields VALUE  Comma separated public fields
  --full  Expand text previews within budgets
  --files  Include bounded changed file names
```

## run.tree

Traverse bounded snapshot execution graph.

```sh
teamcity-axi run tree 482193 --server=work --json
```

```text
teamcity-axi run tree <runId>
Traverse bounded snapshot execution graph

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --depth VALUE  Maximum snapshot traversal depth
  --max-nodes VALUE  Maximum unique executions
```

## run.failure

Investigate independent failure evidence.

```sh
teamcity-axi run failure 482193 --server=work --json
```

```text
teamcity-axi run failure <runId>
Investigate independent failure evidence

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --depth VALUE  Maximum snapshot traversal depth
  --max-nodes VALUE  Maximum unique executions
  --full  Expand text previews within budgets
  --max-diagnosed-runs VALUE  Maximum diagnosed executions including root
```

## run.watch

Observe one execution until terminal or deadline.

```sh
teamcity-axi run watch 482193 --server=work --json
```

```text
teamcity-axi run watch <runId>
Observe one execution until terminal or deadline

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --interval VALUE  Poll interval (ms, s or m)
  --check  Assert terminal success
```

## job.list

Read scoped jobs.

```sh
teamcity-axi job list --project=Payments --server=work --json
```

```text
teamcity-axi job list
Read scoped jobs

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --limit VALUE  Maximum requested rows
  --cursor VALUE  Opaque bounded continuation
```

## job.view

Read safe exact job metadata.

```sh
teamcity-axi job view Payments_Build --server=work --json
```

```text
teamcity-axi job view <id>
Read safe exact job metadata

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
```

## queue.list

Read scoped queued executions.

```sh
teamcity-axi queue list --job=Payments_Build --server=work --json
```

```text
teamcity-axi queue list
Read scoped queued executions

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --limit VALUE  Maximum requested rows
  --cursor VALUE  Opaque bounded continuation
```

## agent.list

Read scoped agent availability.

```sh
teamcity-axi agent list --job=Payments_Build --server=work --json
```

```text
teamcity-axi agent list
Read scoped agent availability

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --limit VALUE  Maximum requested rows
  --cursor VALUE  Opaque bounded continuation
  --pool VALUE  Exact agent pool ID
```

## agent.view

Read safe exact agent metadata.

```sh
teamcity-axi agent view 7 --server=work --json
```

```text
teamcity-axi agent view <id>
Read safe exact agent metadata

  --help  Show local command help
  --version  Show application version
  --format VALUE  Output serialization
  --json  Alias for --format json
  --server VALUE  Registered trusted server alias
  --cwd VALUE  Resolve worktree context from this directory
  --timeout VALUE  Overall deadline (ms, s or m)
  --max-bytes VALUE  Serialized stdout byte ceiling
  --require-complete  Fail if the requested answer is partial
  --no-hints  Omit optional read suggestions
  --debug  Redacted metadata on stderr
  --project VALUE  Exact project ID
  --job VALUE  Exact job ID
  --pool VALUE  Assert exact agent pool ID
```
