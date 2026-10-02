# Controlled TeamCity sandbox fixtures

The captured server is official TeamCity 2026.2 build 238924 with a disposable
HSQLDB database, dedicated Docker volumes and a resource-bounded official minimal
build agent. Its API port is bound only to localhost. The user explicitly approved
accepting its temporary test license. Server and agent image digests are recorded
in the live contract. Administrator setup and fixture mutations are test-only;
product reads and captures use the separate restricted reader.

Create `AxiContract` and `AxiDenied` synthetic projects. Remove inherited All Users
roles and enable per-project permissions. Give `axi-contract-reader` only
`PROJECT_VIEWER` on `AxiContract`. Its captured permission inventory contains
project viewing for that project and its root plus its own profile permission;
it has no build-run permission. Store its token and server URL in an owned 0600
JSON file outside the repository. Do not put administrator credentials in product
configuration. See the existing fixture README for read-only capture commands.

Use command-line build steps for the failed, green and denied jobs. The failed
job emits one synthetic test failure and a build problem, then exits one. The
green and foreign-project jobs exit zero. The captured occurrence IDs and run IDs
belong to this fixture instance; reconstructing a sandbox requires updating its
fixture identity inventory and re-recording sanitized observations.

For positive VCS evidence, initialize an isolated Git repository with `main`,
synthetic author identity and unsigned commits. Add a baseline `fixture.txt` and
clone a bare repository. Copy that bare repository into the disposable server's
data volume; change ownership only for this new fixture directory. Serve only
that directory with Git daemon on the server's private sandbox Docker-network
address. Use `--strict-paths`, a narrow repository argument and that private listen
address; publish no host port. The server image supplies Git. The minimal agent
has no Git executable, so configure server-side checkout.

Create a `jetbrains.git` VCS root `AxiContract_Git` in `AxiContract`, with anonymous
authentication, `refs/heads/main` and the sandbox Git daemon URL. Local file-fetch
URLs were explicitly rejected by TeamCity's security policy; no security setting
was relaxed. Create `AxiContract_Vcs`, bind the root via `vcs-root-entries`, set
`checkoutMode=ON_SERVER`, and add a script that reads `fixture.txt` and exits zero.
Add its snapshot dependency on `AxiContract_Green` using a `snapshot_dependency`
object, `source-buildType` and an empty properties list.

After a successful baseline build, add three synthetic commits with multiline
messages and edited `fixture.txt`, update only the fixture bare repository, and
run the VCS job again. The captured changed run is 9 and directly depends on run 8. Its change page returns commit IDs 3, 2 and 1 under `AxiContract_Git`; requesting
files returns `fixture.txt`. Exact scoped dependency counts are one for run 9 and
zero for run 8. These establish positive direction, paging and VCS-root identity.
They do not establish shared-DAG, cycle, multi-root or hostile-branch support.

The project run inventory now includes synthetic runs 4–9 in addition to 1 and 2;
run 5 is the retained failed local-file-URL setup attempt. Tests assert the exact
current inventory and aggregate outcomes. Repeated setup must inspect existing
objects and live execution state before starting additional builds; do not blindly
replay mutation scripts or assume transient polling failure means a build stopped.

Cleanup applies only to this sandbox's owned containers, Git daemon, dedicated
network and data/log volumes after integration work finishes. It must not remove
unrelated Docker resources. No administrator bootstrap tools or live credentials
are bundled in the product package.

For positive queue evidence, create only two new synthetic jobs
`AxiContract_QueueA` and `AxiContract_QueueB` in the existing allowed project.
Give each a harmless command-line step and an `equals` agent requirement with
`property-name=system.agent.name` and an exact fixture-only unavailable name.
Read back the requirement before enqueueing once with the test administrator.
Do not disable the shared agent. Inspect existing fixture jobs/executions before
any repeated setup; never blindly enqueue again. The retained queued executions
are 10 and 11. The restricted reader observes their queued lifecycle, provider
wait reason, timestamp, project/job scopes and paging. It also reads exact queued
detail with absent result status. The finished-run inventory remains 1/2/4–9;
direct-project job inventory additionally contains the two queue jobs. These
fixture writes are test setup and are absent from product commands.
