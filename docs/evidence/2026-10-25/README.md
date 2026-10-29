# Evidence snapshot — 2026-10-25

This directory records the local proof-boundary audit. `audit.json` separates
fresh command results from historical experiment reports. `artifacts.sha256`
and `source.sha256` bind the inspected files to their bytes; they do not certify
the original author, execution date, or independent reproducibility of a report.

See [the interpretation and limits](../../EVIDENCIAS_E_LIMITES.md).

`summarize.ps1` checks numerical identities with .NET BigInteger and verifies
the small public CRT classes by exhaustive enumeration, independently of the
Go certificate verifier. Use `-OutputDirectory <new-directory>` to preserve
this snapshot when repeating it; `-FreshCRTDirectory` accepts reports from
the three public CRT executables. Without it, only saved CRT reports are used.

The original experiment reports are preserved. Build outputs and a fresh
`.mbvmdb` were created in a temporary directory. No paid model requests,
training jobs, application execution, commits, or deployments were performed.
