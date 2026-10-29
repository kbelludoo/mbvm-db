# Four-tool comparison plan — frozen before new backend measurements

Scope: MBVM-DB impact-server, gopls, Meta Glean, and a complete Sourcegraph
instance with precise SCIP navigation. scip-go is additionally validated as
the shared indexing component; it is not relabelled as a Sourcegraph server.

Use the three frozen source copies and the independently labelled fixture from
the previous comparison. Preserve the 38 selected questions and seven trials
per question, with three complete launches per eligible backend. Hash sources,
tool binaries/images, configuration and protocols. Keep caches outside modules.

Use the same OS/architecture/hardware and Go build configuration for a ranking.
Do not combine native Windows latency with Linux/container latency into one
league table. When Linux is available, rerun MBVM and gopls there as controls.
Retain installation errors, unsupported operations and accuracy failures.

The common task is the previously defined set of direct, non-self callers of
package-level functions in production sources of the local module. Normalize
references using source syntax, not a whitelist of MBVM edges. Real project
expected sets measure agreement; literal fixture labels supply independent
truth. Preserve all backend responses and adapter costs.

Correctness and saved-edit freshness are prerequisites for a performance winner.
For snapshot indexes, measure the documented rebuild/import path needed to make
the changed fact visible; do not query an old index and treat it as current.
Keep startup/indexing, source-update latency and warm query latency separate.

Primary performance criterion: equal weight to each of the three real projects,
geometric mean of warm median latencies across projects, then median across
three full launches. Publish every per-project result, initialization cost,
update cost, and p95 as well. A difference under 5% or inconsistent launch
ordering does not establish a unique performance winner. The toy fixture is
for correctness and is not weighted as a fourth real dataset.

Any conclusion is restricted to this task. No claim about overall database
quality, neural learning, all Go features, or product-wide superiority follows.
If a required full backend cannot be run, explicitly mark the comparison
incomplete; installation unavailability is not a performance defeat.

Local preflight: this Windows host has no Docker/Podman, working WSL distro or
active hypervisor. Glean's documented build target is Linux. The user has offered
an SSH-accessible Linux machine and indicated that keys are in local folders;
the exact SSH destination is pending. Do not display private key material or
send it to the remote server. Do not expose test services publicly.
