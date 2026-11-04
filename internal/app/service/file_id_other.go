//go:build !unix

package service

import "io/fs"

// fileIDOf는 이 운영체제에서 파일의 장치와 inode를 얻지 않는다. renamedFrom은 kqueue에서만 쓴다.
func fileIDOf(fs.FileInfo) (fileID, bool) { return fileID{}, false }
