// Package worktree archives the current working tree of the repository for
// runners that build and test it in another machine, such as a container or
// a virtual machine.
package worktree

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Root returns the top directory of the repository that contains dir.
func Root(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Files lists the tracked files and the untracked files that are not ignored.
// A tracked file deleted in the working tree is still listed; Archive skips it.
func Files(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\x00"), nil
}

// Archive writes the working tree content of files as a tar stream whose
// entries are under src/. Files missing from the working tree are skipped, so
// uncommitted modifications, additions, and deletions are all reflected.
func Archive(out io.Writer, root string, files []string) error {
	archive := tar.NewWriter(out)
	for _, name := range files {
		if name == "" {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = "src/" + filepath.ToSlash(name)
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(archive, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return archive.Close()
}
