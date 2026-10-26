# Closed-loop dependency learning — development protocol v1

The first run uses the existing real `pkg/mbvmdb` Go package, seeds **42, 43,
44**, **5** proposals per acquisition caller, **5,000** initial SGD steps and
**1,000** incremental/control steps. These settings were fixed before observing
the real-package result. Do not tune them against the final file holdout.

## Task and information boundaries

Predict a **direct, statically resolved, same-package call** whose relation is
withheld from the predictor. This is source-relation recovery, not a new program
behavior, a mathematical discovery, or an alternative to parsing available code.

1. Load the existing `internal/impact` typed index. Deduplicate caller/callee
   pairs; multiple call sites count as one relation.
2. Assign source files by seeded SHA-256 ordering: 60% bootstrap, approximately
   20% acquisition, remainder final evaluation. Require at least five files with
   functions and 6..512 symbols; do not reroll a failed split.
3. Bootstrap examples include ONLY pairs whose two endpoints are bootstrap
   symbols. No acquisition or evaluation symbol is a training endpoint.
4. Propose top-5 links for EVERY acquisition caller, including callers with no
   true outgoing static local calls. Candidates are bootstrap/acquisition symbols
   only. Final-test symbols are excluded in both endpoint roles.
5. Independently reload original sources using `hdccsr.LoadTypedRealGraph`.
   Match exact package/receiver/function identity and record source positions.
   Both extractors share Go/types; this is independence from the predictor and
   masked index, not independence from the Go type checker implementation.
6. Persist confirmed positive facts separately from checked negative observations
   in a versioned binary sidecar. Refuse source/module/workspace drift. Read the
   sidecar back BEFORE forming incremental examples.
7. Continue the trained MLP with bootstrap replay and verified acquisition
   observations. The matched-step control continues the same original model
   with bootstrap data only. Both receive exactly 1,000 further SGD steps.
8. Evaluate on final callers against ALL package function candidates. Test
   metadata is allowed for scoring; test labels never influence fitting,
   acquisition, hyperparameters or checkpoints.

The metadata representation is fixed before training: symbol names, receivers,
signatures and same-file information. Bodies, call expressions, graph degrees,
hidden neighbors and identity hashes are excluded. Deterministic HDC8192 encodes
metadata tokens. The MLP has 12 inputs, eight tanh units and one sigmoid output
(113 parameters). Balanced-class SGD uses learning rate 0.03 and weight decay
0.0001. Scores are uncalibrated ranking scores. No MiniLM checkpoint is used.

## Arms

- Frozen random-initialized MLP, with identical architecture/initial seed.
- Fixed HDC similarity without training.
- Fixed metadata overlap/receiver/file rules.
- Trained MLP before acquisition.
- Same MLP after equally budgeted extra training WITHOUT acquisition evidence.
- Same MLP after training on persisted acquisition evidence plus replay.
- Full-source typed extraction reference, explicitly given the complete source
  relation labels. Its information access differs; it is an exact-source upper
  reference, not a competing missing-edge predictor with equal observations.

## Measurements and conclusions

Report query-averaged Recall@5, Precision@5, MRR and MAP. Queries without true
local targets are counted separately and excluded from ranking averages.
Acquisition verification counts include ALL proposed pairs, including failures.
Persist per-query metrics, data partitions, source/build fingerprints, parent
hashes, verified evidence and model weights. Preserve older runs and revisions.

Compute paired recall/MRR changes against BOTH the before-update and matched
extra-training controls. Include a 500-resample file-cluster bootstrap interval
for recall delta. The interval is descriptive on small public dependent data.
Do not pool the related seed splits as independent unseen repositories.

`positive_gain_seeds` counts a strictly positive Recall@5 delta against both
controls. It is a diagnostic count, not a significance test or a success gate.
The program must finish successfully with zero gain, regression or tied results.
An interval containing zero, disagreement across seeds, or poorer performance
than a fixed baseline does not support a general superiority claim.

Verified observations include positive and negative examples. Any incremental
gain is attributed to that joint new information, not automatically to positive
facts alone. The full-source extractor already resolves these static relations
when source is available; neural discovery is not required for source analysis.

## Persistence boundary

`MBVMCL1` is an experimental JSON payload inside a length/CRC32 binary envelope,
with exclusive creation and file sync. It is NOT a replacement for core
`.mbvmdb`, authenticated storage, a WAL, or a crash-durability claim.

- Revision 1: verified bootstrap facts and trained model version 1.
- Revision 2: acquired observations/facts, still model version 1, persisted
  before incremental learning (`verified-before-training.mbvmcl`).
- Revision 3: the same verified knowledge and updated model version 2.

Each later revision records the SHA-256 of its parent. Negative observations
remain observations; they never become positive facts. A high model score
cannot create a fact without a confirming typed-source observation.
