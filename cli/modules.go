package cli

import "github.com/andreyvit/rocketwind"

func (options Options) moduleLoader(files rocketwind.Filesystem, dependency func(string)) rocketwind.ModuleLoader {
	if options.ResolveModule == nil {
		return files.LoadModule
	}
	return func(id, base, kind string) (rocketwind.Module, error) {
		module, err := options.ResolveModule(id, base, kind, dependency)
		if err != nil {
			return rocketwind.Module{}, err
		}
		if module != nil {
			return *module, nil
		}
		return files.LoadModule(id, base, kind)
	}
}
