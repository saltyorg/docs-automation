package document

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFileAtomic publishes complete file contents without exposing partial writes.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode, overwrite bool) error {
	mode := perm.Perm()
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("destination %s is not a regular file", path)
		}
		if !overwrite {
			return fmt.Errorf("%w: %s", fs.ErrExist, path)
		}
		mode = info.Mode().Perm()
	case errors.Is(err, fs.ErrNotExist):
		// A new destination uses the requested mode.
	case err != nil:
		return fmt.Errorf("inspecting destination %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary file for %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("setting temporary mode for %s: %w", path, err)
	}
	n, err := temp.Write(data)
	if err != nil {
		return fmt.Errorf("writing temporary file for %s: %w", path, err)
	}
	if n != len(data) {
		return fmt.Errorf("writing temporary file for %s: %w", path, io.ErrShortWrite)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("syncing temporary file for %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing temporary file for %s: %w", path, err)
	}

	if overwrite {
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf("replacing %s: %w", path, err)
		}
	} else {
		if err := os.Link(tempPath, path); err != nil {
			return fmt.Errorf("publishing %s: %w", path, err)
		}
		if err := os.Remove(tempPath); err != nil {
			return fmt.Errorf("removing temporary name for %s: %w", path, err)
		}
	}

	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("syncing destination directory for %s: %w", path, err)
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
