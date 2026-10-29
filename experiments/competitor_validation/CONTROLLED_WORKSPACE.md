# Controlled workspace amendment — frozen before clean-copy measurements

During the three v1 launches, the MBVM checkout's warm median rose from about
111 to 132 to 157 ms while correctness remained unchanged. Inspection identified
a harness interference: GOPLSCACHE was placed under the compared checkout's
ignored `output/`, and MBVM fingerprintModule walks every directory except .git.
The growing test cache therefore adds directory traversal work to one engine.
The original measurements are valid observations of that workspace but do not
isolate normal source-workspace API costs. Retain every original report.

Copy the same three projects' tracked and nonignored untracked files into new
temporary directories. This includes the new experiment sources, existing dirty
source contents, module files and embedded assets. Exclude generated ignored
outputs by using git's nonignored file inventory; never reset or edit originals.
Compare the source hashes with v1 and retain any mismatch as inconclusive.

Run the UNCHANGED binaries/protocol three times against these fixed copies.
The output, logs and GOPLSCACHE remain outside all three compared module roots.
Each replication uses a fresh cache directory and new tool processes. Preserve
all three launches, full raw messages, exact answers and failures. No timing
selection or engine implementation change is allowed. The fixture is unchanged.

Report original-workspace results separately from clean-workspace results.
Explain that MBVM directory fingerprint cost is sensitive to generated files
and that go:embed still disables its graph cache for Twitch. Neither issue may
be hidden by renaming the workload or presenting only favorable projects.

These observations do not show whether each additional millisecond was caused
by cache directory traversal: a causal estimate would need a separate controlled
cache-size experiment. They establish why caches must be outside compared roots.
