package rocketwind

import (
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Filesystem loads stylesheets, JSON configuration and registered Go modules.
// Root rebases imported asset URLs to the entry stylesheet's directory. Leave
// it empty to preserve URLs. OnDependency receives files used during compilation.
type Filesystem struct {
	// FS optionally supplies files, for example an embed.FS. Paths are relative
	// to its root. A nil FS uses the operating system filesystem.
	FS            iofs.FS
	Compatibility Compatibility
	Root          string
	Modules       map[string]Module
	OnDependency  func(string)
}

func (fs Filesystem) LoadStylesheet(id, base string) (Stylesheet, error) {
	if sheet, ok := loadBuiltinStylesheet(id); ok {
		return sheet, nil
	}
	name, err := fs.resolve(id, base, ".css")
	if err != nil {
		return Stylesheet{}, err
	}
	data, err := fs.read(name)
	if err != nil {
		return Stylesheet{}, err
	}
	sheet := Stylesheet{Path: name, Base: filepath.Dir(name), Content: string(data)}
	if fs.Root != "" {
		root, base := fs.Root, sheet.Base
		if fs.FS != nil {
			// Virtual filesystem paths must not follow host symlinks or depend
			// on the process working directory when rebasing imported URLs.
			root = filepath.Join(string(filepath.Separator), root)
			base = filepath.Join(string(filepath.Separator), base)
		} else {
			var e error
			root, e = filepath.Abs(root)
			if e != nil {
				return Stylesheet{}, e
			}
			if canonical, e := filepath.EvalSymlinks(root); e == nil {
				root = canonical
			}
		}
		sheet.Content, err = RewriteURLs(sheet.Content, base, root)
	}
	return sheet, err
}

func (fs Filesystem) LoadModule(id, base, kind string) (Module, error) {
	if module, ok := fs.Modules[id]; ok {
		return module, nil
	}
	if kind == "plugin" {
		if module, ok := standardModule(id); ok {
			if fs.Compatibility == Tailwind3 && module.legacy3 != nil {
				module.Legacy = module.legacy3
			}
			return module, nil
		}
	}
	if kind != "config" || filepath.Ext(id) != ".json" {
		return Module{}, fmt.Errorf("%s %q requires a registered Go module; JSON configuration is also supported", kind, id)
	}
	name, err := fs.resolve(id, base, ".json")
	if err != nil {
		return Module{}, err
	}
	data, err := fs.read(name)
	if err != nil {
		return Module{}, err
	}
	var object ConfigObject
	if err := json.Unmarshal(data, &object); err != nil {
		return Module{}, fmt.Errorf("%s: %w", name, err)
	}
	module := Module{Base: filepath.Dir(name), Legacy: object}
	if err := fs.jsonPlugins(object, module.Base, &module); err != nil {
		return Module{}, fmt.Errorf("%s: %w", name, err)
	}
	return module, nil
}

func (fs Filesystem) jsonPlugins(object ConfigObject, base string, module *Module) error {
	if presets, ok := object.Get("presets").([]any); ok {
		for _, preset := range presets {
			if err := fs.jsonPlugins(configObject(preset), base, module); err != nil {
				return err
			}
		}
	}
	if value := object.Get("plugins"); value != nil {
		entries, ok := value.([]any)
		if !ok {
			return fmt.Errorf("JSON plugins must be an array of registered Go plugin names")
		}
		for i, entry := range entries {
			name, ok := entry.(string)
			if !ok {
				return fmt.Errorf("JSON plugins must contain registered Go plugin names")
			}
			plugin, err := fs.LoadModule(name, base, "plugin")
			if err != nil {
				return err
			}
			fn := plugin.Plugin
			if plugin.Configure != nil {
				fn, err = plugin.Configure(nil)
				if err != nil {
					return err
				}
			}
			module.Plugins = append(module.Plugins, plugin.Plugins...)
			if fn != nil {
				module.Plugins = append(module.Plugins, fn)
			}
			if len(plugin.Theme) > 0 && module.Theme == nil {
				module.Theme = map[string]string{}
			}
			for key, value := range plugin.Theme {
				module.Theme[key] = value
			}
			entries[i] = ConfigObject{{"config", plugin.Legacy}}
		}
	}
	return nil
}

func (fs Filesystem) read(name string) ([]byte, error) {
	var data []byte
	var err error
	if fs.FS != nil {
		data, err = iofs.ReadFile(fs.FS, filepath.ToSlash(name))
	} else {
		data, err = os.ReadFile(name)
	}
	if err == nil && fs.OnDependency != nil {
		fs.OnDependency(name)
	}
	return data, err
}

func (fs Filesystem) stat(name string) (iofs.FileInfo, error) {
	if fs.FS != nil {
		return iofs.Stat(fs.FS, filepath.ToSlash(name))
	}
	return os.Stat(name)
}

func (fs Filesystem) canonical(name string) (string, error) {
	if fs.FS != nil {
		return filepath.ToSlash(name), nil
	}
	return filepath.EvalSymlinks(name)
}

func (fs Filesystem) resolve(id, base, extension string) (string, error) {
	if base == "" {
		base = "."
	}
	if fs.FS == nil {
		var err error
		base, err = filepath.Abs(base)
		if err != nil {
			return "", err
		}
	}
	if filepath.IsAbs(id) || strings.HasPrefix(id, ".") {
		name := id
		if !filepath.IsAbs(name) {
			name = filepath.Join(base, name)
		}
		if found := fs.resolveFile(name, extension, 0); found != "" {
			return found, nil
		}
	} else {
		for dir := base; ; dir = filepath.Dir(dir) {
			root := filepath.Join(dir, "node_modules")
			if found := fs.resolvePackage(root, id, extension); found != "" {
				return found, nil
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
		if found := fs.resolveFile(filepath.Join(base, id), extension, 0); found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("could not resolve %q from %q", id, base)
}

func (fs Filesystem) resolveFile(name, extension string, depth int) string {
	if depth > 16 {
		return ""
	}
	for _, candidate := range []string{name, name + extension} {
		if info, err := fs.stat(candidate); err == nil && info.Mode().IsRegular() {
			if real, err := fs.canonical(candidate); err == nil {
				return real
			}
		}
	}
	if data, err := fs.read(filepath.Join(name, "package.json")); err == nil {
		var pkg struct{ Style string }
		if json.Unmarshal(data, &pkg) == nil && pkg.Style != "" {
			if found := fs.resolveFile(filepath.Join(name, pkg.Style), extension, depth+1); found != "" {
				return found
			}
		}
	}
	if info, err := fs.stat(filepath.Join(name, "index"+extension)); err == nil && info.Mode().IsRegular() {
		real, _ := fs.canonical(filepath.Join(name, "index"+extension))
		return real
	}
	return ""
}

func (fs Filesystem) resolvePackage(root, id, extension string) string {
	parts := strings.Split(filepath.ToSlash(id), "/")
	n := 1
	if strings.HasPrefix(id, "@") && len(parts) > 1 {
		n = 2
	}
	packageRoot := filepath.Join(root, filepath.Join(parts[:n]...))
	if data, err := fs.read(filepath.Join(packageRoot, "package.json")); err == nil {
		var pkg ConfigObject
		if json.Unmarshal(data, &pkg) == nil && pkg.Get("exports") != nil {
			key := "."
			if len(parts) > n {
				key = "./" + strings.Join(parts[n:], "/")
			}
			target := cssPackageExport(pkg.Get("exports"), key)
			if target == "" {
				return ""
			}
			return fs.resolveFile(filepath.Join(packageRoot, target), extension, 0)
		}
	}
	return fs.resolveFile(filepath.Join(root, id), extension, 0)
}

func cssPackageExport(value any, key string) string {
	switch v := value.(type) {
	case string:
		if key == "." {
			return v
		}
	case []any:
		for _, entry := range v {
			if target := cssPackageExport(entry, key); target != "" {
				return target
			}
		}
	case ConfigObject:
		if entry := v.Get(key); entry != nil {
			return cssPackageExport(entry, ".")
		}
		best, target := "", ""
		for _, entry := range v {
			pattern := entry.Key
			prefix, suffix, ok := strings.Cut(pattern, "*")
			if ok && strings.HasPrefix(key, prefix) && strings.HasSuffix(key, suffix) && len(key) >= len(prefix)+len(suffix) && len(prefix) > len(best) {
				best = prefix
				target = strings.ReplaceAll(cssPackageExport(entry.Value, "."), "*", key[len(prefix):len(key)-len(suffix)])
			}
		}
		if target != "" {
			return target
		}
		if key == "." {
			for _, entry := range v {
				if entry.Key == "style" || entry.Key == "default" {
					if target := cssPackageExport(entry.Value, key); target != "" {
						return target
					}
				}
			}
		}
	}
	return ""
}
