#!/usr/bin/env python3
"""Run the original Rust filesystem tests, recording real scanner operations.

The wrapper snapshots each fixture tree, including symlinks and file mtimes.
No application filesystem contents outside the tests' temporary roots are read.
"""
import json
import os
import re
from pathlib import Path
import subprocess
import sys

upstream = Path(sys.argv[1]).resolve()
cargo = sys.argv[2] if len(sys.argv) > 2 else "cargo"
raw = Path(".cache/scanner.jsonl").resolve()
raw.parent.mkdir(parents=True, exist_ok=True)
raw.write_text("")
originals = {}

def edit(name, fn):
    p = upstream / name
    originals[p] = p.read_bytes()
    p.write_text(fn(p.read_text()))

try:
    lock = upstream / "Cargo.lock"
    originals[lock] = lock.read_bytes()
    edit("crates/oxide/Cargo.toml", lambda s: s.replace("[dev-dependencies]", '[dev-dependencies]\nserde_json = "1"'))
    wrapper = Path("scripts/scanner-recorder.rs").read_text()
    edit("crates/oxide/tests/scanner.rs", lambda s: s.replace("    use tailwindcss_oxide::*;", "    use tailwindcss_oxide::*;\n" + wrapper))
    unit_wrapper = wrapper.replace("tailwindcss_oxide::", "crate::").replace("struct Scanner {", "pub(crate) struct Scanner {")
    unit_wrapper = re.sub(r"(?m)^    fn ", "    pub(crate) fn ", unit_wrapper)
    edit("crates/oxide/src/lib.rs", lambda s: s + '\n#[cfg(test)] mod __rocketwind { use std::path::{Path,PathBuf}; use std::fs; use crate::*;\n' + unit_wrapper + '\n}\n')
    edit("crates/oxide/src/scanner/mod.rs", lambda s: s.replace("use super::{ChangedContent, Scanner};", "use super::ChangedContent; use crate::__rocketwind::Scanner;"))
    edit("crates/oxide/src/glob.rs", lambda s: s.replace("pub fn optimize_patterns(", "fn original_optimize_patterns(") + r'''
pub fn optimize_patterns(entries: &Vec<GlobEntry>) -> Vec<GlobEntry> {
    let result = original_optimize_patterns(entries);
    #[cfg(test)]
    if std::thread::current().name().is_some_and(|n| n.starts_with("glob::tests::")) {
        let json = |entries: &Vec<GlobEntry>| serde_json::json!(entries.iter().map(|g| serde_json::json!({"base":g.base,"pattern":g.pattern})).collect::<Vec<_>>());
        crate::__rocketwind::Scanner::capture("optimize_globs", entries.iter().map(|g| g.base.clone()).collect(), json(entries), json(&result));
    }
    result
}
''')

    def sources(s):
        s = s.replace("pub fn public_source_entries_to_private_source_entries(", "fn original_public_source_entries_to_private_source_entries(")
        s = s.replace("pub fn optimize(&mut self)", "fn original_optimize(&mut self)")
        s = s.replace("fn path_to_posix_string(path:", "fn original_path_to_posix_string(path:")
        return s + r'''
impl PublicSourceEntry {
    pub fn optimize(&mut self) {
        #[cfg(test)] let before = self.clone();
        self.original_optimize();
        #[cfg(test)]
        if std::thread::current().name().is_some_and(|n| n.starts_with("scanner::sources::tests::optimize_")) {
            let json = |s: &PublicSourceEntry| serde_json::json!({"base":s.base,"pattern":s.pattern,"negated":s.negated});
            crate::__rocketwind::Scanner::capture("normalize_source", vec![before.base.clone()], json(&before), json(self));
        }
    }
}
fn path_to_posix_string(path: &Path) -> String {
    let result = original_path_to_posix_string(path);
    #[cfg(test)]
    if std::thread::current().name().is_some_and(|n| n.starts_with("scanner::sources::tests::path_to_posix_")) {
        crate::__rocketwind::Scanner::capture("posix_path", vec![], serde_json::json!(path), serde_json::json!(result));
    }
    result
}
pub fn public_source_entries_to_private_source_entries(sources: Vec<PublicSourceEntry>) -> Vec<SourceEntry> {
    let result = original_public_source_entries_to_private_source_entries(sources.clone());
    #[cfg(test)]
    if std::thread::current().name().is_some_and(|n| n.starts_with("scanner::sources::tests::")) {
        let output = serde_json::json!(result.iter().map(|s| match s {
            SourceEntry::Auto{base} => serde_json::json!({"base":base,"kind":"auto","pattern":"/**/*"}),
            SourceEntry::External{base} => serde_json::json!({"base":base,"kind":"external","pattern":"/**/*"}),
            SourceEntry::Pattern{base,pattern} => serde_json::json!({"base":base,"kind":"pattern","pattern":pattern}),
            SourceEntry::Ignored{base,pattern} => serde_json::json!({"base":base,"kind":"ignored","pattern":pattern}),
        }).collect::<Vec<_>>());
        crate::__rocketwind::Scanner::capture("prepare_sources", sources.iter().map(|s|s.base.clone()).collect(), serde_json::json!(sources.iter().map(|s|serde_json::json!({"base":s.base,"pattern":s.pattern,"negated":s.negated})).collect::<Vec<_>>()), output);
    }
    result
}
'''
    edit("crates/oxide/src/scanner/sources.rs", sources)
    subprocess.run([cargo, "test", "-p", "tailwindcss-oxide", "--lib", "--test", "scanner", "--", "--test-threads=1"], cwd=upstream,
                   env={**os.environ, "ROCKETWIND_CAPTURE": str(raw)}, check=True)
    cases = {}
    for line in raw.read_text().splitlines():
        record = json.loads(line)
        case = cases.setdefault((record["test"], record.pop("id")), {"test": record["test"], "roots": [], "states": [], "steps": []})
        for root in record.pop("roots"):
            if root not in case["roots"]:
                case["roots"].append(root)
        del record["test"]
        case["steps"].append(record)
    for case in cases.values():
        text = json.dumps(case["steps"])
        for i, root in enumerate(case["roots"]):
            text = text.replace(root, f"$ROOT{i}")
            # macOS tempfile paths use /var, while canonical paths use /private/var.
            if root.startswith("/private/"):
                text = text.replace(root[len("/private"):], f"$ROOT{i}")
        case["roots"] = len(case["roots"])
        case["steps"] = json.loads(text)
        times = {}
        for step in case["steps"]:
            if step["method"] in ("files", "scanned_files"):
                # Canonical and /var aliases now identify the same fixture path.
                step["result"] = sorted(set(step["result"]))
            files = step.pop("files")
            for file in files:
                if "mtime" in file:
                    file["mtime"] = times.setdefault(file["mtime"], len(times)+1)
            if files not in case["states"]:
                case["states"].append(files)
            step["state"] = case["states"].index(files)
    integration = [c for c in cases.values() if len(c["test"].split("::")) == 2]
    unit = [c for c in cases.values() if c not in integration]
    Path("testdata/upstream/scanner.json").write_text(json.dumps(integration, ensure_ascii=False, indent=2)+"\n")
    Path("testdata/upstream/scanner-internals.json").write_text(json.dumps(unit, ensure_ascii=False, indent=2)+"\n")
    print(f"Captured {len(cases)} scanner instances, {sum(len(c['steps']) for c in cases.values())} calls")
finally:
    for file, content in originals.items():
        file.write_bytes(content)
