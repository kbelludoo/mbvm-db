# AGENTS.md — Go 1.27 Solo Agent & MBVM-DB Standard

Universal configuration following the open `agents.md` specification.
Canonical specification: `AGENTS.mbvm`.

## Overview
- **Language**: Go 1.27 (Solo autonomous agent execution, zero external orchestrators).
- **Context Engine**: MBVM-DB Smart Slicing (99% token reduction over raw files).
- **Rule of Thumb**: Prioritize `mbvm-db slice . <symbol> 2` before reading entire packages.

## Commands
- **Context Slice**: `mbvm-db slice . <symbol> 2`
- **Inspect Bytecode**: `mbvm-db inspect .`
- **Test Suite**: `go test -race -count=1 -v ./...`
- **Static Vet**: `go vet ./...`
- **Security Audit**: `govulncheck ./...`
- **Pre-commit Compile**: `go-tools mbvm .`

## Architecture & Go Conventions
- **Errors**: Always wrap with context using `%w` (`fmt.Errorf("context: %w", err)`). Never use `%v` for errors.
- **Consumer-side Interfaces**: Define interfaces in the package that consumes them (*"accept interfaces, return structs"*).
- **Goroutine Leak Prevention**: Every goroutine must have a deterministic exit path listening to `<-ctx.Done()`.
- **Modern Stdlib**: Use `slog` for structured logging, `slices`/`maps`, `unique` for value interning, and `iter` for range-over-func.
- **Modularity**: Strict 400 LOC ceiling per file. Refactor into cohesive submodules near the limit.

## Never-Do List (Guardrails)
- Never omit `-race` or `-count=1` when validating tests.
- Never use `time.Sleep` for concurrency synchronization in tests.
- Never introduce `interface{}` or `any` without strict codec justification.
- Never commit with `--no-verify`; the global pre-commit hook must validate AST integrity.
