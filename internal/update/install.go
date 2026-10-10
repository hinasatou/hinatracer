package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// IsNSISInstall reports a Windows install made by mygo's NSIS installer:
// it writes Uninstall.exe next to the app executable.
func IsNSISInstall(exe string) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(exe), "Uninstall.exe"))
	return err == nil
}

// IsSystemInstall reports a Linux binary installed by a package (.deb)
// under /usr or /opt, or any binary whose directory is not writable.
func IsSystemInstall(exe string) bool {
	clean := filepath.ToSlash(filepath.Clean(exe))
	if strings.HasPrefix(clean, "/usr/") || strings.HasPrefix(clean, "/opt/") {
		return true
	}
	return !DirWritable(filepath.Dir(exe))
}

// DirWritable reports whether a file can be created in dir.
func DirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".hinatracer-wtest-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// BundleRoot returns the enclosing .app bundle of a macOS executable path
// (…/HinaTracer.app/Contents/MacOS/HinaTracer), or "".
func BundleRoot(exe string) string {
	p := filepath.Clean(exe)
	for {
		if strings.HasSuffix(strings.ToLower(p), ".app") {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
}

// OldPath is where the running executable is moved during replacement.
func OldPath(exe string) string { return exe + ".old" }

// CleanupOld removes a leftover exe.old (or .app.old) from a previous update.
func CleanupOld(path string) {
	_ = os.RemoveAll(OldPath(path))
}

// ReplaceFile swaps target for newFile using the rename trick, which works
// for a running executable on Windows: target -> target.old, newFile ->
// target. On failure the original is restored. The .old file is removed
// later (CleanupOld) since a running exe cannot be deleted on Windows.
func ReplaceFile(target, newFile string) error {
	mode := os.FileMode(0o755)
	if st, err := os.Stat(target); err == nil {
		mode = st.Mode().Perm()
	}
	old := OldPath(target)
	_ = os.RemoveAll(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("move current executable aside: %w", err)
	}
	if err := moveFile(newFile, target, mode); err != nil {
		if rerr := os.Rename(old, target); rerr != nil {
			return fmt.Errorf("%v (restore failed: %v)", err, rerr)
		}
		return err
	}
	return nil
}

// ReplaceDir swaps a directory (macOS .app bundle) the same way.
func ReplaceDir(target, newDir string) error {
	old := OldPath(target)
	_ = os.RemoveAll(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("move current app aside: %w", err)
	}
	if err := os.Rename(newDir, target); err != nil {
		if rerr := os.Rename(old, target); rerr != nil {
			return fmt.Errorf("%v (restore failed: %v)", err, rerr)
		}
		return err
	}
	_ = os.RemoveAll(old)
	return nil
}

func moveFile(src, dst string, mode os.FileMode) error {
	if err := os.Rename(src, dst); err == nil {
		return os.Chmod(dst, mode)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(src)
	return nil
}

// ErrNotInArchive means the expected file is missing from the archive.
var ErrNotInArchive = errors.New("file not found in archive")

// ExtractZipFile extracts the first regular file whose base name matches
// one of names (case-insensitive) into dst.
func ExtractZipFile(zipPath string, names []string, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(filepath.FromSlash(f.Name))
		for _, n := range names {
			if strings.EqualFold(base, n) {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				defer rc.Close()
				return writeFile(dst, rc, 0o755)
			}
		}
	}
	return fmt.Errorf("%w: %v", ErrNotInArchive, names)
}

// ExtractTarGzFile extracts the first regular file with base name name.
func ExtractTarGzFile(tgzPath, name, dst string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(h.Name) == name {
			return writeFile(dst, tr, 0o755)
		}
	}
	return fmt.Errorf("%w: %s", ErrNotInArchive, name)
}

// ExtractZipApp extracts the *.app bundle from a macOS zip into destDir and
// returns its path. Modes and symlinks are preserved; paths are confined.
func ExtractZipApp(zipPath, destDir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	root := ""
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		first := strings.SplitN(filepath.ToSlash(name), "/", 2)[0]
		if !strings.HasSuffix(strings.ToLower(first), ".app") {
			continue
		}
		if root == "" {
			root = first
		}
		target := filepath.Join(destDir, name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return "", fmt.Errorf("unsafe path in archive: %s", f.Name)
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case mode&os.ModeSymlink != 0:
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			link, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			if err := os.Symlink(string(link), target); err != nil {
				return "", err
			}
		default:
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0o644
			}
			err = writeFile(target, rc, perm)
			rc.Close()
			if err != nil {
				return "", err
			}
		}
	}
	if root == "" {
		return "", fmt.Errorf("%w: .app", ErrNotInArchive)
	}
	return filepath.Join(destDir, root), nil
}

func writeFile(dst string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
