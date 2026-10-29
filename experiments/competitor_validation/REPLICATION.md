# Replication plan — recorded after v1 run and before replication

The first formal report is retained at
`output/gopls-comparison-2026-10-26/report.json`; the fixture-only pilot at
`output/gopls-comparison-pilot-2026-10-26/report.json` is also retained. The pilot
was used to validate adapters, not select results or an alternative workload.

First formal run: both tools matched every selected caller set. Observed real
project medians: MBVM 110.545 / gopls 102.285 ms; Twitch 726.791 / 48.429 ms;
Kick 41.131 / 59.476 ms. The fixture timing differed from the earlier pilot.

Run the unchanged v1 protocol two more times with independent tool process
launches, new output directories and a new task-local GOPLSCACHE for each run.
Keep binaries, sources, query selection, Go environment and seven-round trial
count unchanged. Retain all three runs, including errors and slower samples;
do not pick a preferred run. Compare real source hashes and binary hashes.

Report all three medians and correctness counts. Treat inconsistent ordering
as variable, not a general speed win. This is still a single host and small
fixed workload, not a product-wide ranking. The first report's protocol hash
continues to identify the unchanged PROTOCOL.md; this additional document
explains why and how the two replications were added.
