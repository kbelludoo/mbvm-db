# AGENTS.md — Nim, Go & Rust Solo Agent Standard (MBVM-DB)

Universal configuration following the open `agents.md` specification.
Canonical specification: `AGENTS.mbvm`.

## Overview
- **Primary Runtime Language**: **Nim (v2.2+)**. Primary core engine for zero-allocation CSR traversal, HDC 8K vectors, and minimal RSS (< 5 MB).
- **Secondary / Legacy Ecosystems**: Go 1.27 & Modern Rust (Stable 2021/2024 Edition). Auto-detected via `nim.cfg`, `go.mod`, or `Cargo.toml`.
- **Mode**: Solo autonomous agent execution (single AI, direct action, zero external orchestrators).
- **Context Engine**: MBVM-DB Smart Slicing (99% token reduction over raw files).
- **Single-Ingestion Law**: Heavy assets (Protobuf, JSON catalogs, AST, game data) are ingested ONCE into `.mbvmdb` / `.mbvm8k2` binary databases. The AI NEVER reads raw 50k+ token files. Always query `mbvm-db slice <db> <term/symbol> [depth]` (60–200 tokens, 1-3ms).

## Commands by Ecosystem
### Nim Ecosystem (`nim.cfg`, `mbvm.nimble`) — Primary Core
- **Build Release CLI**: `nim c -d:release src/mbvm.nim`
- **Unit Test Suite**: `nim c -r tests/test_csr.nim` and `nim c -r tests/test_hdc.nim`
- **Static Syntax Check**: `nim check src/mbvm.nim`
- **Benchmark Run**: `./src/mbvm bench`

### Go Ecosystem (`go.mod`)
- **Context Slice**: `mbvm-db slice <db_or_dir> <symbol_or_term> [depth]`
- **Test Suite**: `go test -race -count=1 -v ./...`
- **Static Vet**: `go vet ./...`
- **Security Audit**: `govulncheck ./...`
- **Pre-commit Compile**: `go-tools mbvm .`

### Rust Ecosystem (`Cargo.toml`)
- **Check**: `cargo check --workspace --all-targets`
- **Lint (Strict)**: `cargo clippy --workspace --all-targets --all-features -- -D warnings`
- **Format**: `cargo fmt --all --check`
- **Test Suite**: `cargo test --workspace --all-features -- --nocapture`
- **Security Audit**: `cargo audit`

## Architecture & Code Conventions
### Nim Conventions (Core Engine)
- **Memory Model**: MUST strictly use `--mm:orc` (or `--mm:arc`) to guarantee deterministic memory reclamation without background tracing GC pause.
- **Zero-Allocation Traversals**: Traversals and hot loops must accept reusable `var seq[T]` / `ScratchBuffer` instances; 0 B/op in BFS and HDC vector math.
- **Binary Codecs**: Use `std/memfiles` for zero-copy memory-mapped file access.
- **Exceptions**: Use typed exceptions and clean assertions via `std/unittest`.

### Go Conventions
- **Errors**: Always wrap with context using `%w` (`fmt.Errorf("context: %w", err)`). Never use `%v`.
- **Consumer-side Interfaces**: Define interfaces in the package that consumes them (*"accept interfaces, return structs"*).
- **Goroutine Leak Prevention**: Every goroutine must have a deterministic exit path listening to `<-ctx.Done()`.
- **Modern Stdlib**: Use `slog` for structured logging, `slices`/`maps`, `unique` for value interning, and `iter` for range-over-func.

### Rust Conventions
- **Error Handling**: Use `thiserror` for crates/libraries and `anyhow` for binaries/CLIs with `.context(...)`. Always propagate with `?`.
- **Never `.unwrap()` in Production**: Only use `.expect("invariant context")` when an invariant violation is mathematically impossible.
- **Unsafe Boundaries**: Avoid `unsafe`. Any necessary `unsafe` block MUST be preceded by an explicit `// SAFETY:` explanation.
- **Async Runtime**: Never block Tokio worker threads with synchronous file I/O or heavy computation; use `tokio::task::spawn_blocking`.
- **Agent Loops**: Bound all autonomous loops with hard `max_iterations` and timeout bounds.

## Universal Never-Do List (Guardrails)
- Never read raw heavy catalogs, Protobuf definitions, or whole packages when a `.mbvmdb` exists.
- Never use a tracing GC model in Nim; strictly use `--mm:orc` / `--mm:arc`.
- Never omit `-race` or `-count=1` in Go tests.
- Never commit Rust code with warnings or failing `cargo clippy -- -D warnings`.
- Never use `time.Sleep` for concurrency synchronization in tests.
- Never commit secrets, API keys, or `.env` files.
- Never commit with `--no-verify`.
