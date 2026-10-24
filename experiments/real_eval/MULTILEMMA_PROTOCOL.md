# MBVM-DB — Multi-Lemma Evaluation Protocol (development specification)

**Status:** proposed protocol, not an experimental result. No Gemini calls or mathematical discovery are claimed here.

## Research question and null hypothesis

**Question:** Can CSR dependency-aware retrieval select a *smaller, sufficient, independently checkable* set of mathematical premises than a strong conventional inverted index, under an identical proof/composition executor?

**Null:** When both retrievers have the same catalog, same composer, same inference budget and same proof checker, CSR does not improve correctness or certified minimum-context cost.

A shorter CSR slice is not evidence of better reasoning unless correctness and sufficiency are independently verified.

## Strict separation of three tasks

1. **CRT intersection (development control):** Given two independent congruence premises, compute the *unique simultaneous residue class*. This proves information composition, **not** a new Erdős–Straus family. If one premise is removed, the requested unique class modulo the product must become underdetermined.
2. **Erdős–Straus instantiation (arithmetic control):** Given a verified formula with applicable hypotheses, compute an exact positive integer triple. This measures formula application, **not** multi-lemma synthesis.
3. **Genuine mathematical lemma composition (target):** Derive a new explicitly stated theorem, witness or obstruction whose **formal proof under the frozen allowed inference rules** needs two or more independent premises. A rule chain or two matching identifiers alone is not a certificate. In particular, an intersection of two sufficient solution families is **not** evidence that solving Erdős–Straus needs both families.

Report all three families separately. Do not pool their accuracy into a single "math intelligence" score.

## Development CRT controls

For coprime positive moduli `m1,m2 > 1` and canonical residues `0 <= ri < mi`, a certificate must contain:

```json
{
  "goal": "unique_crt_class",
  "premises": [{"id":"L1","modulus":8,"residue":5},{"id":"L2","modulus":9,"residue":7}],
  "certificate": {"residue":61,"modulus":72,"parents":["L1","L2"]}
}
```

The verifier must independently check `M = m1*m2`, `0 <= R < M`, `R mod mi = ri`, and `gcd(m1,m2)=1`. It must also check **premise indispensability for this specific goal**: retaining only L1 or only L2 admits at least two different residues modulo M. This tests the exact intersection objective, not generic problem-solving ability.

Use independent brute-force enumeration for small M as a second oracle. Add negative fixtures for inconsistent non-coprime systems. Do not "verify" an unbounded theorem by checking only 10–20 numerical examples.

## Genuine Erdős–Straus composition criteria

For each target theorem, freeze the proof calculus and the rule catalog before generating the holdout.

- **Soundness:** every transformation has a machine-checkable rule, explicit substitutions, integer divisibility/positivity conditions and parent certificate identifiers.
- **Minimality, relative to the frozen calculus:** remove each supposed indispensable parent in turn; the permitted proof search must fail to derive the *same target* without it. Record search bounds. Bounded failure is **not** a proof of global impossibility.
- **No atomic shortcut:** neither parent, another catalog rule nor an equivalent formula can directly yield the required theorem/witness. Search for these shortcuts before accepting a problem.
- **Correctness:** for numeric triples verify `4*x*y*z = n*(x*y+x*z+y*z)` with `math/big`. For CRT verify the unique residue modulo lcm. For universal statements require symbolic/certificate verification, not substring matching.
- **No conflation:** record separately `retrieved`, `applicable`, `actually_used_in_certificate`, and `indispensable_under_calculus`.

Do not call the existing `ComposeCRT` an independent constructive composition of Erdős–Straus solvers: its current `SolveFunc` tries parent solvers one after another.

## Paired ablation arms

| Arm | Retrieval | Composer | Proof checker |
|---|---|---|---|
| A | None | Same | Same |
| B | Exact/filtered inverted index + conventional multihop expansion | Same | Same |
| C | CSR graph traversal + equivalent semantic pruning | Same | Same |
| D | Oracle minimal relevant premises (upper-bound control only) | Same | Same |

Arm B must be allowed query expansion over explicit premises, dependency identifiers and the same rule descriptions; it must **not** be artificially restricted to one residue-key lookup if the task supplies multiple conditions. Measure retrieval latency, rule count, redundant rule count and context bytes independently of model outcomes. Hold all other generation parameters constant, including model version, temperature, maximum output tokens and system instruction.

Arm D is not a deployable baseline; it estimates the model performance when relevant evidence is selected perfectly. Include a Flat RAG arm only as an additional diagnostic, never as the only conventional control.

## Dataset governance

Public code and fixed examples are **development fixtures only**. Public GitHub source is not a blind holdout. Build a separate, access-restricted holdout *after code/model/prompt freeze*. Record a cryptographic commitment and immutable version metadata; publish it only after evaluation. Exclude all earlier B30/U20 questions and demonstrated examples as unseen claims. Ground-truth proof certificates must be generated independently of the evaluated model and manually audited for nontrivial theorems.

For each held-out task, record goal, frozen calculus ID, allowed premise IDs, exact certificate, shortcut audit, complexity tier and independent reviewer decision. Balance CRT-only, arithmetic instantiation, and genuine proof composition categories. Do not mix their scores.

## Measurement and error handling

Use matched independent trials with the same requested model and store actual returned model version, prompt bodies/hashes, retrieved rule IDs, ordered edges, generation config, latency, tokens (prompt/completion/thoughts/total), raw response, oracle decisions and errors.

- Distinguish model incorrect/unfinished, oracle inconclusive, and HTTP 429/503/timeout. Never count service failure as a mathematics failure or as zero-token usage.
- Average token costs on received model responses. Separately measure request availability and end-to-end task success per issued request.
- Summarize **paired task-level** accuracy differences and token deltas, with uncertainty intervals. One successful response per arm on one n is only anecdotal.
- Predeclare the number of tasks and repeats, keep a stable model, and include failed/inconclusive tasks in the published raw result set.
- **Stop/go:** advance to paid inference only after the independent certificate verifier, negative controls and no-shortcut checks pass locally.

## Expected falsifiable outcomes

- B equals C in correctness, with smaller/faster B retrieval: **no demonstrated CSR advantage**.
- C retrieves more rules but uses none of the additional parents: **context inflation, not composition**.
- C improves task-level certificate validity under matched context budget and the same composer: **evidence for the retrieval topology**, conditional on sufficient repeated trials and uncertainty analysis.
- All arms solve the task with a single lemma: **reject the case as a multi-lemma test**.
