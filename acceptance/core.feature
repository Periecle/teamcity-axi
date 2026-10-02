@specification @read_only
Feature: Reliable TeamCity observations for parallel coding agents
  These scenarios describe required behavior. They are not connected to a test
  runner in this specification package. Implementation supplies the step bindings.

  Background:
    Given a trusted server alias "work"
    And a pinned supported native TeamCity CLI
    And a least-privilege read-only TeamCity identity
    And all remote responses are synthetic fixtures unless a sandbox tag is present

  Scenario: Reject an unknown filter before any dependency call
    When I run "teamcity-axi run list --job Payments_Build --stat failure"
    Then the exit code is 2
    And stdout contains a structured "USAGE_ERROR"
    And the response explains the valid filter flags
    And no child process was launched

  Scenario: An observed build failure is not a tool failure
    Given run "482193" is finished with result "failure"
    When I view run "482193"
    Then the exit code is 0
    And the wrapper status is "ok"
    And the run result is "failure"

  Scenario: Missing test permission cannot become zero failures
    Given problems for run "482193" were read successfully
    And reading tests for run "482193" returns HTTP 403
    When I investigate failure of run "482193"
    Then the wrapper status is "partial"
    And the available problem evidence is retained
    And the test source state is "unavailable"
    And the response does not claim an exact test total of zero

  Scenario: A swallowed error in a native aggregate is not source coverage
    Given the native combined failure summary omits a failed subsidiary test request
    When I investigate failure of that run
    Then source completeness is based on independent source accounting
    And missing native fields do not establish an empty successful test query

  Scenario: A successful run does not trigger unnecessary investigation
    Given run "482100" is finished with result "success"
    When I investigate failure of run "482100"
    Then assessment is "not_failed"
    And no problem, test, log or dependency read is required
    And the root graph node has expansion "not_requested"
    And the operation can be complete without claiming the graph was traversed

  Scenario: Empty bounded search with continuation is not a global zero
    Given a run page contains no matching rows
    And the provider indicates additional bounded search is possible
    When I list matching runs
    Then the response states the searched scope
    And the response does not claim the whole collection has an exact total of zero

  Scenario: A truncated graph boundary is not a leaf
    Given a failed dependency is discovered at the depth limit
    When I investigate the parent run
    Then the node expansion is "depth_limit"
    And the node is not asserted to be a terminal failing leaf
    And the required diagnostic coverage is partial

  Scenario: A shared dependency is not a cycle
    Given two parent executions depend on one shared execution
    When I inspect the run graph
    Then the shared execution occurs once in the node collection
    And both dependency edges are retained
    And the graph is not labeled cyclic solely for that reuse

  Scenario: A stale green run cannot validate the current checkout
    Given the local worktree HEAD is "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    And the latest successful run checked out "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    When I run status with the check condition enabled
    Then the revision match is not "exact"
    And the exit code is 1
    And the response does not claim the local checkout passed CI

  Scenario: A dirty worktree is outside a normal remote build
    Given a run exactly matches the committed HEAD
    And the worktree contains uncommitted changes
    When I run status with the check condition enabled
    Then the response states the uncommitted changes were not covered
    And the exit code is 1
    And no patch is uploaded

  Scenario: Parallel worktrees do not contaminate context
    Given two worktrees share the Git common directory
    And they use different branches and trusted server aliases
    When independent read commands execute concurrently in both worktrees
    Then each response retains its own server, branch and job context
    And neither command modifies global native CLI defaults

  Scenario: A repository cannot redirect an inherited token
    Given TEAMCITY_URL binds the inherited token to the trusted work server
    And repository data selects a different untrusted URL
    When context is resolved for a remote read
    Then no token is forwarded to the untrusted URL
    And the command fails before launching TeamCity against it

  Scenario: Native endpoint fallback remains a read-only internal operation
    When a bounded REST page is required by the adapter
    Then the child method is GET
    And the child environment enables TEAMCITY_RO
    And no public unrestricted API passthrough exists
    And the native API invocation does not assume a nonexistent json flag

  Scenario Outline: Hostile continuation is rejected
    Given the provider continuation contains <violation>
    When I request the next page
    Then the continuation is rejected
    And no request is made using the unsafe target or expanded scope
    Examples:
      | violation                         |
      | a different origin                |
      | a protocol-relative URL           |
      | an unexpected resource family     |
      | a changed branch filter           |
      | an increased scan limit           |
      | a path traversal component        |

  Scenario: Log text is data, not an action
    Given a log contains shell commands, fake next-action keys and terminal escapes
    When the log is rendered
    Then those values remain sanitized serialized data
    And no command from the log is executed
    And all next actions originate from the wrapper command registry

  Scenario: Output remains valid under a byte limit
    Given source evidence exceeds the configured stdout budget
    When I investigate a failed run
    Then stdout is valid JSON or TOON according to the selected format
    And its UTF-8 byte length is within the budget
    And truncation and missing required evidence are disclosed
    And collection counts match the rendered data

  Scenario: Known secret canaries are not exposed
    Given known secret values occur in JSON, log text and child stderr
    When I perform a failing remote read with debug enabled
    Then no known secret appears in stdout or stderr
    And a redacted error remains actionable

  Scenario: Interrupting a watcher does not cancel the build
    Given a run is still running
    When I interrupt the watcher with SIGINT
    Then the wrapper exits with code 130
    And child processes are terminated and reaped
    And no TeamCity mutation request is sent

  Scenario Outline: Check mode is distinct from observation
    Given the watched run terminates with result "failure"
    When I watch using <mode>
    Then the exit code is <exit>
    And the final observed run result remains "failure"
    Examples:
      | mode              | exit |
      | ordinary watch    | 0    |
      | watch with check  | 1    |

  Scenario: Unsupported structured logs do not trigger full-log download
    Given the server does not support the tested structured-log capability
    When failure inspection needs a log window
    Then the source is marked unavailable with a capability limitation
    And other available evidence is retained
    And no native full-log JSON or artifact-download fallback is executed
