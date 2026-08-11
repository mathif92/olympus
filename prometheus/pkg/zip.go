package pkg

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxCodeBytes caps the size of an uploaded code archive.
const MaxCodeBytes = 10 << 20 // 10 MiB

// unsafeZipEntry rejects entries that escape the target directory (absolute
// paths, parent traversal) regardless of path separator.
func unsafeZipEntry(name string) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") {
		return true
	}
	if name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") || strings.HasSuffix(name, "/..") {
		return true
	}
	return false
}

// ValidateFunctionCode ensures code is a readable zip archive of bounded size,
// contains no path-escaping entries, and includes every file the runtime
// requires as its entrypoint (the language's "restriction to be executed").
func ValidateFunctionCode(code []byte, rt Runtime) error {
	if len(code) > MaxCodeBytes {
		return fmt.Errorf("code archive too large: %d bytes (max %d)", len(code), MaxCodeBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(code), int64(len(code)))
	if err != nil {
		return fmt.Errorf("invalid zip archive: %w", err)
	}
	have := make(map[string]bool)
	for _, f := range zr.File {
		if unsafeZipEntry(f.Name) {
			return fmt.Errorf("unsafe zip entry %q", f.Name)
		}
		have[filepath.ToSlash(f.Name)] = true
	}
	for _, req := range rt.RequiredFiles {
		if !have[req] {
			return fmt.Errorf("runtime %s requires %q in the archive", rt.ID, req)
		}
	}
	return nil
}

// extractZip writes every entry of the archive under dst, rejecting unsafe
// paths, and returns nil on success.
func extractZip(code []byte, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(code), int64(len(code)))
	if err != nil {
		return err
	}
	base := filepath.Clean(dst) + string(filepath.Separator)
	for _, f := range zr.File {
		if unsafeZipEntry(f.Name) {
			return fmt.Errorf("unsafe zip entry %q", f.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, base) {
			return fmt.Errorf("unsafe zip entry %q escapes the workspace", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			rc.Close()
			return err
		}
		w, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			w.Close()
			rc.Close()
			return err
		}
		if err := w.Close(); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}
