# Public 3-Premise CRT Multi-hop Development Lab

This is **not** a hidden benchmark and **not** a new Erdős–Straus theorem.
It is a controlled regression for retrieval through a **two-stage chain**
of three independently necessary CRT congruence premises.

## Architecture

Every public development task has the same topology:

```text
FINAL
├── MID
│   ├── L1
│   └── L2
└── L3
```

- `MID` contains only `Dependencies=[L1,L2]`, never a stored CRT answer.
- `FINAL` contains only `Dependencies=[MID,L3]`, never a stored CRT answer.
- Two conflicting modular distractors are in the shared catalog but are **not**
  dependencies of this proof. Both retrieval arms must avoid them.
- **B:** conventional ID index + recursive dependency expansion;
  **C:** CSR `MathCSRGraph.Neighbors` traversing `RelDependency`.
- Both return exactly the same three leaves to the same exact `math/big`
  two-stage CRT executor and verifier.
- A separate exhaustive small-modulus oracle checks uniqueness modulo
  `M=m1*m2*m3` and checks that removing **any** premise leaves more than
  one possible residue modulo `M`.

## Run locally

```bash
go test -count=1 -run '^TestCRTDepth' ./experiments/real_eval
go run ./cmd/run_multilemma_depth_dev --report=crt_depth_dev_results.json
go vet ./experiments/...
```

These 12 tasks are public, synthetic and **intentionally easy for an exact
algorithm**. Retrieval latency per single task is diagnostic only, not a valid
speed benchmark. The CSR is not expected to win when a strong conventional
index has the same explicit dependencies.

## What this does and does not test

| Claim | Tested here? |
|---|---|
| Recursive two-hop dependency traversal reaches all three leaves | Yes |
| Two sequential CRT compositions yield an exact, auditable residue class | Yes |
| Removing one of three premises prevents uniqueness modulo product | Yes (finite exhaustive oracle) |
| Irrelevant catalog entries are excluded by both retrievers | Yes |
| CSR improves answer quality over a strong conventional index | No |
| CSR semantic pruning beats equivalent conventional pruning | No |
| Gemini inference consumes fewer reasoning tokens | No API calls |
| Generalized new Erdős–Straus solution family is discovered | No |

The distractors are deliberately not proof dependencies. Excluding them is
**dependency filtering**, not evidence that CSR alone has superior semantic
pruning. The next scientific test must supply both methods with the same
relevance metadata and evaluate cases where genuinely alternative proof
paths, irrelevant branches and non-atomic conclusions are present.

For the eventual external model evaluation, follow `MULTILEMMA_PROTOCOL.md`,
freeze a separate private holdout **after** the source and prompts are stable,
and publish the hash commitment before revealing the holdout.
