package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Backups contain only the app's records, linked bodies and undo history.
// No subprocess ever removes data. Restore rejects traversal and special files.
func (s *Store) backup(to string, daily bool) (string, error) {
	if e := os.MkdirAll(to, 0700); e != nil {
		return "", e
	}
	name := s.now().Format("2006-01-02T150405.000000000Z") + ".tar.gz"
	if daily {
		name = s.now().Format("2006-01-02") + ".tar.gz"
	}
	path := filepath.Join(to, name)
	if daily {
		if _, e := os.Stat(path); e == nil {
			return path, nil
		}
	}
	f, e := os.CreateTemp(to, ".pending-backup-*")
	if e != nil {
		return "", e
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, root := range []string{"q", ".history"} {
		e = filepath.WalkDir(filepath.Join(s.Dir, root), func(p string, d os.DirEntry, e error) error {
			if errors.Is(e, os.ErrNotExist) {
				return nil
			}
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() || strings.HasPrefix(d.Name(), ".pending-") {
				return nil
			}
			rel, e := filepath.Rel(s.Dir, p)
			if e != nil {
				return e
			}
			if e = tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0600, Size: info.Size(), ModTime: info.ModTime()}); e != nil {
				return e
			}
			in, e := os.Open(p)
			if e != nil {
				return e
			}
			defer in.Close()
			_, e = io.Copy(tw, in)
			return e
		})
		if e != nil {
			break
		}
	}
	e = errors.Join(e, tw.Close(), gz.Close(), f.Sync(), f.Close())
	if e != nil {
		return "", e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return "", e
	}
	if daily && s.BackupKeep > 0 {
		files, e := filepath.Glob(filepath.Join(to, "*.tar.gz"))
		if e != nil {
			return "", e
		}
		for len(files) > s.BackupKeep {
			if e = os.Remove(files[0]); e != nil {
				return "", e
			}
			files = files[1:]
		}
	}
	return path, nil
}
func (s *Store) Backup(to string) (out string, err error) {
	if to == "" {
		to = filepath.Join(s.Dir, "backups")
	}
	err = s.locked(func() error { var e error; out, e = s.backup(to, false); return e })
	return
}
func (s *Store) Restore(path string) error {
	if e := s.permit("restore", Question{}); e != nil {
		return e
	}
	return s.locked(func() error {
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		defer f.Close()
		gz, e := gzip.NewReader(f)
		if e != nil {
			return e
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		var rows []Question
		files := map[string][]byte{}
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			parts := strings.Split(h.Name, "/")
			if len(parts) != 2 || parts[0] != "q" || parts[1] == "" || parts[1] == ".." || strings.HasPrefix(parts[1], ".") || h.Typeflag != tar.TypeReg {
				if strings.HasPrefix(h.Name, ".history/") && h.Typeflag == tar.TypeReg {
					continue
				}
				return fmt.Errorf("unsafe backup member %q", h.Name)
			}
			b, e := io.ReadAll(io.LimitReader(tr, 128<<20))
			if e != nil {
				return e
			}
			if int64(len(b)) != h.Size {
				return errors.New("backup member too large")
			}
			if strings.HasSuffix(h.Name, ".json") {
				var q Question
				if e = jsonDecode(b, &q); e != nil {
					return e
				}
				if !safeQuestion(q) {
					return errors.New("invalid question in backup")
				}
				rows = append(rows, q)
			} else {
				files[parts[1]] = b
			}
		}
		old, e := s.all()
		if e != nil {
			return e
		}
		for name, b := range files {
			if e = atomicBytes(filepath.Join(s.Dir, "q", name), b); e != nil {
				return e
			}
		}
		ids := map[string]bool{}
		for _, q := range rows {
			ids[q.ID] = true
		}
		for _, q := range old {
			if !ids[q.ID] {
				q.Deleted = true
				rows = append(rows, q)
			}
		}
		return s.commit(old, rows, "restore")
	})
}
