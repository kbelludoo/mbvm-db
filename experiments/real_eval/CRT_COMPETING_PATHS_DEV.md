# CRT competing proof paths — PUBLIC development control

Status: implementation for review. **No Gemini inference, no new Erdős–Straus
theorems, and no assertion that CSR improves mathematical accuracy.**

Extends `CRT_DEPTH_DEV.md` with a shared proof objective and **four competing
plans** for each of the same 12 public CRT tasks.

```text
GOAL (OR)
├── SHORT     (AND) -> [L1, L2, L3]             VALID: 5 trace nodes
├── LONG      (AND) -> [MID, L3]                VALID: 6 trace nodes
│                          MID -> [L1, L2]
├── BAD       (AND) -> [NOISE1, L2, L3]         INVALID: NOISE1 contradicts query
└── OVERLOAD  (AND) -> [MID, L3, NOISE2]        INVALID: NOISE2 contradicts query
```

The query value is derived from the independent, existing three-stage CRT
development certificate. Every node is accessible to both retrieval arms.
None of the new plan nodes stores a solved residue.

## Fair B/C arms

- **B:** ID-indexed recursive dependencies from the same `Rule.Dependencies`.
- **C:** the actual CSR `MathCSRGraph.Neighbors` with `RelDependency`.
- Identical AND/OR plan selector, exact modular applicability checks,
  tie-breaking, 3-lemma CRT executor and mathematical certificate verifier.
- Both must choose SHORT, retain exactly L1/L2/L3, reject two incompatible
  plans, and produce byte-identical context and matching certificates.
- Disabling SHORT must yield LONG; disabling both valid plans must reject
  the query, rather than accepting a contradictory branch.

## Run locally

```sh
go test -count=1 -run '^TestCRTChoice' ./experiments/real_eval
go run ./cmd/run_multilemma_choice_dev --report=crt_choice_dev_results.json
go vet ./experiments/...
```

JSON records the selected plan, rule IDs, traversed proof nodes, number of
inapplicable alternatives, certificate, context bytes and individual retrieval
timings. Single-call retrieval timings are **diagnostic only** and cannot
establish a throughput or latency advantage.

## Interpretation and limitations

1. The graph edges and OR/AND types are supplied explicitly to both arms.
   Selecting the shortest compatible alternative is **not automatic discovery**
   of previously unknown mathematical relationships.
2. Invalid branches are rejected because their modular premises contradict the
   supplied query. This tests equivalent constraint pruning but **does not**
   prove a CSR-specific semantic-filtering capability.
3. The minimal trace is minimal **among these four enumerated plans only**,
   by unique node count; this is not a global proof-minimality claim.
4. A conventional indexed adjacency graph can have equivalent mathematical
   capabilities to CSR. CSR's distinctive benefit may lie in memory layout
   and traversal cost at scale, which requires a separate, properly warmed,
   allocation-aware benchmark with equivalent indexing.
5. These public fixtures are development tests, not an unseen/private holdout.
   Any subsequent actual Gemini evaluation must follow
   `MULTILEMMA_PROTOCOL.md`, freeze the prompt and catalog first and retain
   the independent arithmetic proof checker.
