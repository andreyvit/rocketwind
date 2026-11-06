#!/usr/bin/env python3
"""Run original Rust extractor tests and record their inputs and results.

Only the temporary upstream checkout is instrumented. Its files are restored
after the test run. Rust and serde_json are development tools, not Go dependencies.
"""
import json
import os
from pathlib import Path
import subprocess
import sys

upstream = Path(sys.argv[1]).resolve()
cargo = sys.argv[2] if len(sys.argv) > 2 else "cargo"
output = Path("testdata/upstream/oxide.json").resolve()
raw = Path(".cache/oxide.jsonl").resolve()
raw.parent.mkdir(parents=True, exist_ok=True)
raw.write_text("")
originals = {}


def edit(name, fn):
    file = upstream / name
    originals.setdefault(file, file.read_bytes())
    file.write_text(fn(file.read_text()))


recorder = r'''
#[cfg(test)]
pub fn rocketwind_record(kind: &str, input: &[u8], output: serde_json::Value) {
    use std::io::Write;
    let Ok(path) = std::env::var("ROCKETWIND_CAPTURE") else { return };
    static LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());
    let _guard = LOCK.lock().unwrap();
    let mut file = std::fs::OpenOptions::new().create(true).append(true).open(path).unwrap();
    let mut value = serde_json::json!({"kind":kind,"input":String::from_utf8_lossy(input),"result":output});
    if std::str::from_utf8(input).is_err() { value["inputBytes"] = serde_json::json!(input); }
    writeln!(&mut file,"{}",value).unwrap();
}
'''

try:
    lock = upstream / "Cargo.lock"
    originals[lock] = lock.read_bytes()
    edit("crates/oxide/Cargo.toml", lambda s: s.replace("[dev-dependencies]", '[dev-dependencies]\nserde_json = "1"'))
    edit("crates/oxide/src/lib.rs", lambda s: s + recorder)

    def machines(s):
        start = s.index("    fn test_extract_all(")
        before, body = s[:start], s[start:]
        body = body.replace("        input\n", "        let actual: Vec<&str> = input\n", 1)
        body = body.replace("            .collect()", '''            .collect();
        crate::rocketwind_record(std::any::type_name::<Self>(), input.as_bytes(), serde_json::json!(actual));
        actual''', 1)
        return before + body

    edit("crates/oxide/src/extractor/machine.rs", machines)

    def extractors(s):
        for kind in ["extract", "css-variables"]:
            pos = s.index("        extracted\n", s.index("pub fn extract" if kind == "extract" else "pub fn extract_variables_from_css"))
            code = r'''        #[cfg(test)]
        crate::rocketwind_record("KIND", self.cursor.input, serde_json::json!(extracted.iter().map(|item| {
            let (kind,bytes) = match item { Extracted::Candidate(bytes) => ("candidate",*bytes), Extracted::CssVariable(bytes) => ("variable",*bytes) };
            let start = bytes.as_ptr() as usize - self.cursor.input.as_ptr() as usize;
            serde_json::json!({"kind":kind,"value":String::from_utf8_lossy(bytes),"start":start,"end":start+bytes.len()})
        }).collect::<Vec<_>>()));
'''.replace("KIND", kind)
            s = s[:pos] + code + s[pos:]
        return s

    edit("crates/oxide/src/extractor/mod.rs", extractors)

    processors = upstream / "crates/oxide/src/extractor/pre_processors"
    for file in processors.glob("*.rs"):
        if file.name in ("pre_processor.rs", "mod.rs"):
            continue
        edit(str(file.relative_to(upstream)), lambda s: s.replace("fn process(", "fn process_original("))
    edit("crates/oxide/src/extractor/pre_processors/pre_processor.rs", lambda s: s.replace(
        "    fn process(&self, content: &[u8]) -> Vec<u8>;", r'''
    fn process_original(&self, content: &[u8]) -> Vec<u8>;
    fn process(&self, content: &[u8]) -> Vec<u8> {
        let result = self.process_original(content);
        #[cfg(test)]
        crate::rocketwind_record(&format!("preprocess:{}",std::any::type_name::<Self>()),content,match std::str::from_utf8(&result) { Ok(text) => serde_json::json!(text), Err(_) => serde_json::json!({"bytes":result}) });
        result
    }'''))
    subprocess.run([cargo, "test", "-p", "tailwindcss-oxide", "--lib", "extractor", "--", "--test-threads=1"], cwd=upstream,
                   env={**os.environ, "ROCKETWIND_CAPTURE": str(raw)}, check=True)
    unique = {}
    for line in raw.read_text().splitlines():
        record = json.loads(line)
        record["kind"] = record["kind"].replace("tailwindcss_oxide::extractor::", "")
        unique[json.dumps(record, sort_keys=True)] = record
    records = sorted(unique.values(), key=lambda r: (r["kind"], r["input"]))
    output.write_text(json.dumps(records, ensure_ascii=False, indent=2) + "\n")
    Path("testdata/upstream/example.html").write_bytes((upstream / "crates/oxide/src/fixtures/example.html").read_bytes())
    print(f"Captured {len(records)} distinct extractor and preprocessor calls")
finally:
    for file, content in originals.items():
        file.write_bytes(content)
