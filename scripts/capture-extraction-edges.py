#!/usr/bin/env python3
"""Compare Go-fuzzer inputs with the pinned Rust extractor and preprocessors.

Usage: capture-extraction-edges.py UPSTREAM CARGO [INPUTS_JSON]
Without INPUTS_JSON, reuse the inputs in the checked-in capture. Rust is only
needed to regenerate these development fixtures, never to build Rocketwind.
"""
import gzip
import json
from pathlib import Path
import subprocess
import sys

upstream = Path(sys.argv[1]).resolve()
cargo = sys.argv[2]
output = Path("testdata/upstream/extraction-edges.json.gz")
inputs = json.loads(Path(sys.argv[3]).read_text()) if len(sys.argv) > 3 else [r["input"] for r in json.loads(gzip.decompress(output.read_bytes()))]
cache = Path(".cache/extraction-edges/reference").resolve()
(cache / "src").mkdir(parents=True, exist_ok=True)
(cache / "inputs.json").write_text(json.dumps(inputs))
(cache / "Cargo.toml").write_text(f'''[package]
name = "rocketwind-extraction-edges"
version = "0.0.0"
edition = "2021"
[dependencies]
tailwindcss-oxide = {{ path = {json.dumps(str(upstream / "crates/oxide"))} }}
serde_json = "=1.0.145"
''')
(cache / "src/main.rs").write_text(r'''
use tailwindcss_oxide::extractor::{Extractor, Extracted, pre_processors::*};
use serde_json::{json, Value};
fn main() {
    let inputs: Vec<Vec<u8>> = serde_json::from_str(include_str!("../inputs.json")).unwrap();
    let mut records = Vec::new();
    for input in inputs {
        let mut outputs = serde_json::Map::new();
        macro_rules! process { ($name:literal, $p:ty) => {
            outputs.insert($name.into(), json!(<$p>::default().process(&input)));
        }; }
        process!("clj", Clojure); process!("ex", Elixir); process!("haml", Haml);
        process!("json", Json); process!("md", Markdown); process!("pug", Pug);
        process!("razor", Razor); process!("rb", Ruby); process!("rs", Rust);
        process!("slim", Slim); process!("svelte", Svelte); process!("tt", TemplateToolkit);
        process!("twig", Twig); process!("vue", Vue);
        let mut extracted = Vec::<Value>::new();
        for token in Extractor::new(&input).extract() {
            let (kind, value) = match token { Extracted::Candidate(value) => ("candidate", value), Extracted::CssVariable(value) => ("variable", value) };
            let start = value.as_ptr() as usize - input.as_ptr() as usize;
            extracted.push(json!({"kind":kind,"start":start,"end":start+value.len()}));
        }
        records.push(json!({"input":input,"outputs":outputs,"extracted":extracted}));
    }
    println!("{}", serde_json::to_string(&records).unwrap());
}
''')
result = subprocess.run([cargo, "run", "--quiet", "--manifest-path", str(cache / "Cargo.toml")], check=True, stdout=subprocess.PIPE)
records = json.loads(result.stdout)
output.write_bytes(gzip.compress((json.dumps(records, separators=(",", ":"))+"\n").encode(), mtime=0))
print(f"Captured {len(records)} Rust extraction and {len(records)*14} preprocessing cases")
