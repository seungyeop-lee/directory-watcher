// Package worktree는 저장소의 현재 작업 트리를, 컨테이너나 가상 머신처럼 다른
// 기기에서 빌드하고 테스트하는 실행기를 위해 아카이브로 묶는다.
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

// Root는 dir을 포함하는 저장소의 최상위 디렉터리를 돌려준다.
func Root(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Files는 추적 중인 파일과, 추적하지 않지만 무시되지도 않는 파일을 나열한다. 작업
// 트리에서 지운 추적 파일도 목록에 남으며, Archive가 이 파일을 건너뛴다.
func Files(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\x00"), nil
}

// Archive는 files의 작업 트리 내용을, 항목이 src/ 아래에 놓이는 tar 스트림으로 쓴다.
// 작업 트리에 없는 파일은 건너뛰므로, 커밋하지 않은 수정과 추가, 삭제가 모두
// 반영된다.
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
