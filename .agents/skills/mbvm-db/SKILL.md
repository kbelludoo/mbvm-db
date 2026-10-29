---
name: mbvm-db
description: >-
  Query, inspect, and slice Go codebases, AST, and protocol definitions using MBVM-DB to achieve 90%+ token reduction.
  Use this skill whenever the user mentions mbvm-db, slicing code, inspecting codebase architecture, finding functions/types,
  or analyzing typed impact instead of searching the web or reading raw files.
---

# MBVM-DB Context Engine Skill

MBVM-DB (Micro-Bytecode Virtual Machine Database) is the high-performance context engine for codebases and protocol schemas.
It indexes Go AST into compressed binary databases (`codebase.mbvmdb`) to allow ultra-fast subgraph slicing (1-3ms) with 90-99% token reduction compared to reading entire source files.

> [!IMPORTANT]
> **NEVER search the web** when `mbvm-db` is mentioned. `mbvm-db` is the local binary CLI tool (`mbvm-db.exe` or `mbvm-db`) or native MCP server inside this project.
> Always use `mbvm-db slice` rather than reading raw heavy files or packages.

---

## Available Commands & Usage

Run these commands using the terminal (`run_command`):

### 1. Context Slicing (Primary AI Workflow)
Extracts a minimal sub-graph of symbols, types, and dependencies around a target symbol:
```powershell
.\mbvm-db.exe slice codebase.mbvmdb <symbol_or_func> [depth]
```
* **depth**: Default is `2` (traverses callers/callees/types 2 hops away).
* If `codebase.mbvmdb` does not exist yet, pass `.` as the target:
  ```powershell
  .\mbvm-db.exe slice . <symbol_or_func> [depth]
  ```

### 2. Token-Budgeted Slicing
Extracts a subgraph constrained strictly under a token ceiling to eliminate noise:
```powershell
.\mbvm-db.exe slice-budget codebase.mbvmdb <symbol> <max_tokens> [depth]
```
* Example: `.\mbvm-db.exe slice-budget codebase.mbvmdb LoadFromFile 500 3`

### 3. Inspect Codebase & Database Status
Inspects the total files, indexed symbols, and bytecode volume:
```powershell
.\mbvm-db.exe inspect codebase.mbvmdb
```

### 4. Typed Impact Analysis
Analyzes call graphs, blast radius, and type contract impacts:
```powershell
.\mbvm-db.exe impact -dir ./pkg/mbvmdb -symbol <symbol>
```

### 5. Build / Re-index AST
Compiles the current workspace AST into a fresh indexed binary database:
```powershell
.\mbvm-db.exe build . codebase.mbvmdb
```

### 6. Verify Database Integrity
Validates CRC32 checksum and magic headers of a `.mbvmdb` binary:
```powershell
.\mbvm-db.exe verify codebase.mbvmdb
```

---

## Workflow Rules
1. **Query First**: Always run `.\mbvm-db.exe slice codebase.mbvmdb <symbol>` first before opening multiple `.go` files.
2. **Deterministic Context**: Use the output of `slice` as the primary context for code reasoning, refactoring, and bug fixes.
3. **No Web Fallback**: If a symbol is not found in the slice, try with depth `3` or run `.\mbvm-db.exe inspect` to list project symbols.
