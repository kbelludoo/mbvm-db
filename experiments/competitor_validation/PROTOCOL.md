# Persistent gopls comparison v1 — fixed before execution

Question: how do the shipped MBVM impact-server and gopls v0.24.0 compare on
repeated queries for distinct direct callers of package-level Go functions?
This is a bounded comparison of existing APIs, not neural learning, a complete
Go analysis benchmark, or a comparison against Glean/Sourcegraph deployments.

## Fixed workload

- The same local MBVM-DB, twitch-miner-go and kick-miner-go modules used in the
  previous value study. Retain failures; do not substitute projects.
- One independently labelled fixture exercises cross-package calls, methods as
  callers, closures, go/defer, generic instantiation, function values, recursion,
  unused functions and test-only references. Only this temporary fixture is edited.
- Build-selected production source, named enclosing functions, module-local
  callers; omit the target itself to match the current MBVM impact API.
- Targets are non-init package-level functions, not concrete/interface methods.
  This avoids pretending that interface-expanded references equal static calls.
  Up to six targets with callers and six without, selected by ascending SHA-256
  of `competitor-v1:qualified-name`, before observing either tool's timings.
- Expected real-project sets come from the existing typed MBVM graph and are
  agreement checks, not independent truth. The fixture's literal expected sets
  are independent of both tools. Do not conflate these two forms of evidence.

## gopls adapter

Use stdio LSP initialize/initialized and prepareCallHierarchy/incomingCalls.
Persist one process per module. Preserve all protocol messages. Resolve target
locations and prepare hierarchy items before warm timings. gopls incomingCalls
uses references, so independently parse the returned source ranges and keep
only identifiers in the callee expression of a CallExpr inside a production
FuncDecl. Unwrap parentheses and generic indexes. Do not filter using MBVM edges
or expected caller sets. Count and retain all discarded ranges. Source parsing
and normalization are included in end-to-end query timing, using a prepared
source catalogue shared across samples; raw gopls RPC time is also reported.

## Measurement

- Both tools use the same Go environment, pinned binaries and source snapshot.
  Go compiler/export caches may already be warm. No claim of cold disk caches.
- Record source and binary hashes, Go settings, version output and raw samples.
- Separate process start + first useful answer, target preparation, and warm
  queries. Warm every target once, then seven full rounds, alternating the order
  of the two processes. Same targets, equal trial counts. Keep slow/zero gains.
- Record exact caller sets, missing/extra names, median and nearest-rank p95,
  transport reply bytes, cache stats, and full source hashes before and after.
- MBVM also computes transitive candidates in its existing response. This is
  actual API latency, not an isolated equal-work graph algorithm comparison.
- No RSS or index-size superiority claims: this first test does not capture
  total peak process-tree memory or comparable persistent indexes.
- Freshness check: in the labelled temporary fixture, change a saved caller
  from Target to Unused; send LSP didOpen/didChange/didSave, then query both.
  Record save/notification delivery cost separately. Measure each next query
  after notifications were sent; this is one saved-edit check, not a general
  measurement of editor event delivery or asynchronous indexing convergence.
- No positive result is required. Only make speed claims where correctness is
  established for the stated scope. Preserve failed and mismatched queries.

## Reproduction

Install gopls with a task-local GOBIN, without changing the project's go.mod:
`go install golang.org/x/tools/gopls@v0.24.0`.
Build `./cmd/mbvm-db` and `./experiments/competitor_validation/cmd/compare`.
Run the comparator with `-mbvm`, `-gopls`, `-out` (a new directory) and optional
semicolon-separated `-roots`. It never runs the compared applications or uploads
their source. Keep the raw directory with the resulting report.
