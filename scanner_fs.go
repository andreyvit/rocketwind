package rocketwind

import (
	"io/fs"
	"path"
	"strings"
)

// ScanFS scans text assets in embedded or virtual filesystems without reading
// the host filesystem. It applies Tailwind's default file exclusions, including
// binary assets, CSS, lockfiles and dependency directories. Sources configured
// for disk scanning do not apply. Results include all previously seen candidates.
func (s *Scanner) ScanFS(filesystems ...fs.FS) ([]string, error) {
	batch := scanBatch{scanner: s}
	err := s.scanFS(&batch, filesystems)
	batch.finish()
	if err != nil {
		return nil, err
	}
	return s.sortedCandidates(), nil
}

func (s *Scanner) scanFS(batch *scanBatch, filesystems []fs.FS) error {
	for _, files := range filesystems {
		err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path.Ext(name) == ".css" || scanIgnoreMatch(scanDefaultIgnores, name, entry.IsDir()) == 1 {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			data, err := fs.ReadFile(files, name)
			if err != nil {
				return err
			}
			s.addContent(Content{Text: string(data), Extension: strings.TrimPrefix(path.Ext(name), ".")}, true, nil, batch)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
