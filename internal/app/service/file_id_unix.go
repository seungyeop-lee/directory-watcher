//go:build unix

package service

import (
	"io/fs"
	"syscall"
)

// fileIDOf는 info가 가리키는 파일의 장치와 inode를 돌려준다.
func fileIDOf(info fs.FileInfo) (fileID, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, false
	}
	return fileID{dev: uint64(st.Dev), ino: uint64(st.Ino)}, true
}
