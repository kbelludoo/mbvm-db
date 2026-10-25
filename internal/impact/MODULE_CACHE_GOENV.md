# Module cache — persistent Go environment hardening

The **per-process** cache is already implemented in `internal/impact/module_cache.go`.
It stores private `ModuleGraph` snapshots behind `ModuleCache.Impact(root, symbol)`
and returns a **new report** per query. It is intentionally *not* a
`LoadModuleCached() *ModuleGraph` API: exposing the cached pointer would
allow accidental mutation of typechecked graph internals.

## Why GOENV must be fingerprinted

`go env -w GOFLAGS=-tags=release` writes configuration outside the
environment of the parent process. A later `go list` reads those persistent
settings while `os.Getenv("GOFLAGS")` can still be empty. A fingerprint
based only on `os.Getenv` and module files may then incorrectly reuse a
typed graph selected under different build tags.

The additional `fingerprintGoEnv` helper hashes the **contents** of the
active configuration file. It handles the explicit `GOENV` path,
`GOENV=off`, and the default `os.UserConfigDir()/go/env`. A missing
file has a stable sentinel distinct from a subsequently created file.
File read failures disable caching rather than silently treating an
unobserved input as unchanged.

The `replace` guard also uses tokenized first-field matching to catch
legal whitespace-separated directives. Local replacements remain
an explicit bypass.

## Tests

~~~sh
go test -race -count=1 -run 'TestModuleCache' ./internal/impact
go vet ./internal/impact/...
go test -run '^$' -bench '^BenchmarkModuleCacheWarmImpact$' -benchmem -count=5 ./internal/impact
~~~

The new regression performs an **identical-length edit with restored mtime**
to the persistent GOENV file, checks the cache reloads once, then verifies
the next query hits the new key. The test also verifies file creation and
`GOENV=off`, plus whitespace-separated replace directives.

## Boundaries

- A cache hit still scans and hashes source files: **do not claim 1–5 ms**
  without measuring the actual module on the target machine.
- A cache is beneficial only for **multiple queries in one process**.
  The standalone `mbvm-db impact-module` CLI starts fresh every time; no
  cross-process reuse is claimed.
- A change during a warm hit after the fingerprint read is not an
  atomic filesystem snapshot. External concurrent editors may require a
  caller-level retry or filesystem snapshot strategy.
- Remote package cache content, Go compiler auto-downloads, and source
  settings outside the fingerprint's scope require separate analysis.
  The fingerprint is conservative but not a universal build provenance
  certificate; do not describe it as one.
