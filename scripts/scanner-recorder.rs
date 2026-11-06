// Test-only wrapper inserted into the original upstream scanner test module.
struct Scanner {
    inner: tailwindcss_oxide::Scanner,
    roots: Vec<PathBuf>,
    id: u64,
}

impl Scanner {
    fn capture(method: &str, bases: Vec<String>, args: serde_json::Value, result: serde_json::Value) {
        static ID: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(1000000);
        let mut this = Self { inner: tailwindcss_oxide::Scanner::default(), roots: vec![], id: ID.fetch_add(1, std::sync::atomic::Ordering::Relaxed) };
        for base in bases { this.root(Path::new(&base)); }
        this.record(method, args, result);
    }

    fn new(sources: Vec<PublicSourceEntry>) -> Self {
        static ID: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);
        let mut this = Self { inner: tailwindcss_oxide::Scanner::new(sources.clone()), roots: vec![], id: ID.fetch_add(1, std::sync::atomic::Ordering::Relaxed) };
        for source in &sources { this.root(Path::new(&source.base)); }
        this.record("new", serde_json::json!(sources.iter().map(|s| serde_json::json!({"base":s.base,"pattern":s.pattern,"negated":s.negated})).collect::<Vec<_>>()), serde_json::Value::Null);
        this
    }

    fn root(&mut self, path: &Path) {
        for parent in path.ancestors() {
            if parent.file_name().is_some_and(|name| name.to_string_lossy().starts_with(".tmp")) {
                let root = dunce::canonicalize(parent).unwrap_or(parent.to_path_buf());
                if !self.roots.contains(&root) { self.roots.push(root); }
                break;
            }
        }
    }

    fn changed(&mut self, content: &ChangedContent) -> serde_json::Value {
        match content {
            ChangedContent::File(path, ext) => { self.root(path); serde_json::json!({"file":path,"extension":ext}) },
            ChangedContent::Content(text, ext) => serde_json::json!({"content":text,"extension":ext}),
        }
    }

    fn record(&self, method: &str, args: serde_json::Value, result: serde_json::Value) {
        use std::io::Write;
        fn files(path: &Path, out: &mut Vec<serde_json::Value>) {
            let Ok(meta) = fs::symlink_metadata(path) else { return };
            if meta.file_type().is_symlink() {
                out.push(serde_json::json!({"path":path,"kind":"link","target":fs::read_link(path).unwrap()}));
            } else if meta.is_dir() {
                out.push(serde_json::json!({"path":path,"kind":"dir"}));
                if path.file_name().is_some_and(|n| n == ".git") { return }
                let mut entries = fs::read_dir(path).unwrap().filter_map(Result::ok).collect::<Vec<_>>();
                entries.sort_by_key(|e| e.file_name());
                for entry in entries { files(&entry.path(), out); }
            } else if meta.is_file() {
                let data = fs::read(path).unwrap();
                let mut entry = serde_json::json!({"path":path,"kind":"file","content":String::from_utf8_lossy(&data),"mtime":format!("{:?}",meta.modified().unwrap())});
                if std::str::from_utf8(&data).is_err() { entry["bytes"] = serde_json::json!(data); }
                out.push(entry);
            }
        }
        let mut snapshot = vec![];
        for root in &self.roots { files(root, &mut snapshot); }
        let record = serde_json::json!({"id":self.id,"test":std::thread::current().name(),"roots":self.roots,"files":snapshot,"method":method,"args":args,"result":result});
        let mut file = fs::OpenOptions::new().create(true).append(true).open(std::env::var("ROCKETWIND_CAPTURE").unwrap()).unwrap();
        writeln!(&mut file, "{}", record).unwrap();
    }

    fn scan(&mut self) -> Vec<String> {
        let result = self.inner.scan();
        self.record("scan", serde_json::Value::Null, serde_json::json!(result));
        result
    }
    fn scan_content(&mut self, content: Vec<ChangedContent>) -> Vec<String> {
        let args = serde_json::json!(content.iter().map(|c| self.changed(c)).collect::<Vec<_>>());
        let result = self.inner.scan_content(content);
        self.record("scan_content", args, serde_json::json!(result));
        result
    }
    fn get_files(&mut self) -> Vec<String> {
        let mut result = self.inner.get_files(); result.sort();
        self.record("files", serde_json::Value::Null, serde_json::json!(result));
        result
    }
    fn get_scanned_files(&self) -> Vec<String> {
        let result = self.inner.get_scanned_files();
        self.record("scanned_files", serde_json::Value::Null, serde_json::json!(result));
        result
    }
    fn get_globs(&mut self) -> Vec<GlobEntry> {
        let result = self.inner.get_globs();
        self.record("globs", serde_json::Value::Null, serde_json::json!(result.iter().map(|s| serde_json::json!({"base":s.base,"pattern":s.pattern})).collect::<Vec<_>>()));
        result
    }
    fn get_normalized_sources(&self) -> Vec<GlobEntry> {
        let result = self.inner.get_normalized_sources();
        self.record("normalized_sources", serde_json::Value::Null, serde_json::json!(result.iter().map(|s| serde_json::json!({"base":s.base,"pattern":s.pattern})).collect::<Vec<_>>()));
        result
    }
    fn get_candidates_with_positions(&mut self, content: ChangedContent) -> Vec<(String, usize)> {
        let args = self.changed(&content);
        let result = self.inner.get_candidates_with_positions(content);
        self.record("positions", args, serde_json::json!(result));
        result
    }
}
