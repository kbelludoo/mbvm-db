"""Validate official scip-go output; this is not a Sourcegraph query server."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
from urllib.parse import unquote, urlsplit


def key(path, line, character):
    return f"{str(Path(path).resolve()).lower()}:{line}:{character}"


def uri_path(uri):
    parsed = urlsplit(uri)
    if parsed.scheme != "file" or parsed.netloc:
        raise ValueError(f"unsupported URI: {uri}")
    path = unquote(parsed.path)
    if len(path) > 2 and path[0] == "/" and path[2] == ":":
        path = path[1:]
    return Path(path)


def occurrence_start(occurrence):
    typed = occurrence.get("TypedRange") or {}
    if "SingleLineRange" in typed:
        value = typed["SingleLineRange"]
        return value.get("line", 0), value.get("start_character", 0)
    if "MultiLineRange" in typed:
        value = typed["MultiLineRange"]
        return value.get("start_line", 0), value.get("start_character", 0)
    legacy = occurrence.get("range", [])
    if len(legacy) in (3, 4):
        return legacy[0], legacy[1]
    raise ValueError("missing or unsupported SCIP occurrence range")


def utf16_column(lines, line, column, encoding):
    text = lines[line]
    if encoding == 1:
        prefix = text.encode("utf-8")[:column].decode("utf-8")
    elif encoding == 2:
        return column
    elif encoding == 3:
        prefix = text[:column]
    else:
        raise ValueError(f"ambiguous SCIP position encoding {encoding}")
    return len(prefix.encode("utf-16-le")) // 2


def check_index(catalogue, index):
    root = Path(catalogue["root"])
    targets = catalogue["targets"]
    target_positions = {
        key(uri_path(t["uri"]), t["position"]["line"], t["position"]["character"]): t
        for t in targets
    }
    definitions = {t["symbol"]: set() for t in targets}
    occurrences = []
    for doc in index.get("documents", []):
        relative = Path(doc["relative_path"])
        if relative.is_absolute() or ".." in relative.parts:
            raise ValueError("SCIP path escapes selected root")
        path = root / relative
        lines = path.read_text(encoding="utf-8").splitlines()
        encoding = doc.get("position_encoding", 0)
        for occ in doc.get("occurrences", []):
            line, column = occurrence_start(occ)
            column = utf16_column(lines, line, column, encoding)
            pos = key(path, line, column)
            if occ.get("symbol_roles", 0) & 1:
                if pos in target_positions:
                    definitions[target_positions[pos]["symbol"]].add(occ.get("symbol", ""))
            else:
                occurrences.append((pos, occ.get("symbol", "")))
    rows = []
    for target in targets:
        symbols = definitions[target["symbol"]]
        if len(symbols) != 1 or "" in symbols:
            rows.append({"symbol": target["symbol"], "error": "definition missing or ambiguous"})
            continue
        callers, discarded = set(), 0
        for pos, symbol in occurrences:
            if symbol not in symbols:
                continue
            caller = catalogue["call_positions"].get(pos)
            if caller is None or caller == target["symbol"]:
                discarded += 1
            else:
                callers.add(caller)
        expected = set(target["expected_callers"])
        rows.append({
            "symbol": target["symbol"], "scip_symbol": next(iter(symbols)),
            "expected": sorted(expected), "actual": sorted(callers),
            "missing": sorted(expected - callers), "extra": sorted(callers - expected),
            "discarded_noncall_test_or_self_ranges": discarded,
            "exact": callers == expected,
        })
    return rows


def run(command, env, stdout_path, stderr_path):
    start = time.perf_counter()
    with open(stdout_path, "wb") as out, open(stderr_path, "wb") as err:
        result = subprocess.run(command, env=env, stdout=out, stderr=err, timeout=300, check=False)
    if result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {command[0]}; see {stderr_path}")
    return 1000 * (time.perf_counter() - start)


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--tools", default="output/all-competitor-tools")
    args = parser.parse_args()
    output = Path(args.out).resolve()
    output.mkdir()
    tools = Path(args.tools).resolve()
    env = os.environ | {"CGO_ENABLED": "0", "GOMAXPROCS": "1", "GOPROXY": "off",
                        "GOSUMDB": "off", "GOFLAGS": "-mod=readonly", "GOWORK": "off"}
    baseline = json.loads(Path(args.baseline).read_text(encoding="utf-8-sig"))
    report = {
        "schema": "scip-component-accuracy-v1", "scip_go_version": "0.2.9",
        "scip_go_sha256": sha(tools / "scip-go.exe"),
        "scip_cli_source_tag": "v0.10.0", "scip_cli_sha256": sha(tools / "scip.exe"),
        "cases": [], "limits": [
            "This validates scip-go indexes through the official SCIP JSON decoder.",
            "The Python syntax-position adapter is not Sourcegraph or Glean; its timing is not ranked.",
            "Real expected answers are graph agreement, fixture labels are independent.",
        ],
    }
    for number, old in enumerate(baseline["cases"]):
        directory = output / f"case-{number}"
        directory.mkdir()
        root = str(output / "fixture") if number == 0 else old["root"]
        case = {"root": root}
        try:
            cat_path = directory / "catalogue.json"
            command = [str(tools / "scip-catalog.exe"), "-root", root, "-out", str(cat_path)]
            if number == 0:
                command.append("-fixture")
            run(command, env, directory / "catalog.stdout", directory / "catalog.stderr")
            catalogue = json.loads(cat_path.read_text(encoding="utf-8-sig"))
            if catalogue["source_sha256"] != old["source_before"]:
                raise ValueError("source digest differs from frozen baseline")
            if sorted(t["symbol"] for t in catalogue["targets"]) != sorted(t["symbol"] for t in old["targets"]):
                raise ValueError("query selection differs from frozen baseline")
            module_path = (Path(root) / "go.mod").read_text(encoding="utf-8").split()[1]
            index_path = directory / "index.scip"

            def build_index(suffix):
                build_path = directory / f"index{suffix}.scip"
                elapsed = run([
                    str(tools / "scip-go.exe"), "index", "--module-root", root,
                    "--module-path", module_path, "--module-version", "frozen-v1",
                    "--repository-remote", module_path, "--skip-tests", "-o", str(build_path),
                ], env, directory / f"index{suffix}.stdout", directory / f"index{suffix}.stderr")
                decoded = directory / f"index{suffix}.json"
                run([str(tools / "scip.exe"), "print", "--json", str(build_path)], env,
                    decoded, directory / f"decode{suffix}.stderr")
                return elapsed, json.loads(decoded.read_text(encoding="utf-8-sig"))

            elapsed, index = build_index("")
            case |= {"source_sha256": catalogue["source_sha256"], "index_ms": elapsed,
                     "index_bytes": index_path.stat().st_size, "index_sha256": sha(index_path),
                     "queries": check_index(catalogue, index)}
            if number == 0:
                source = Path(root) / "lib.go"
                text = source.read_text(encoding="utf-8")
                source.write_text(text.replace("func Switch() { Target() }", "func Switch() { Unused() }", 1), encoding="utf-8")
                changed = json.loads(json.dumps(catalogue))
                for target in changed["targets"]:
                    if target["symbol"] == "example.com/callfixture.Target":
                        target["expected_callers"].remove("example.com/callfixture.Switch")
                    if target["symbol"] == "example.com/callfixture.Unused":
                        target["expected_callers"] = ["example.com/callfixture.Switch"]
                case["stale_snapshot_queries"] = check_index(changed, index)
                refreshed_ms, refreshed = build_index("-after-save")
                case["reindex_ms"] = refreshed_ms
                case["refreshed_queries"] = check_index(changed, refreshed)
            # Re-export solely to prove real source stability, not to relabel answers.
            if number > 0:
                after_path = directory / "after.json"
                run([str(tools / "scip-catalog.exe"), "-root", root, "-out", str(after_path)], env,
                    directory / "after.stdout", directory / "after.stderr")
                after = json.loads(after_path.read_text(encoding="utf-8-sig"))
                if after["source_sha256"] != catalogue["source_sha256"]:
                    raise ValueError("real source changed during indexing")
                case["sources_unchanged"] = True
        except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
            case["error"] = str(error)
        report["cases"].append(case)
        (output / "report.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
        print(f"Completed SCIP case {number}; preserved report and raw index", flush=True)


if __name__ == "__main__":
    main()
