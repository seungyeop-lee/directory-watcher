package service

import (
	"os"
	"path/filepath"
	"strings"
)

// entryInfo는 감시 중인 폴더 안에 있는 항목 하나의 기록이다.
type entryInfo struct {
	// id는 kqueue에서만 채운다. renamedFrom을 본다.
	id  fileID
	dir bool
	// excluded는 exclude나 기본 제외에 걸린 항목이다. fsnotify의 kqueue 구현은 이런 항목에도 감시를 열므로,
	// unwatchTree가 그 감시를 풀 수 있게 kqueue에서만 적어 두고 recorded는 기록에 없는 항목으로 본다.
	excluded bool
}

// recorded는 감시 대상 안의 경로 path가 기록에 있는지와 폴더인지를 돌려준다.
//
// 기록은 감시를 시작할 때 감시 대상을 훑어 채우고, 항목의 생성과 삭제, 이름 바꾸기, 옮겨 오기와 옮겨 나가기
// 이벤트를 받을 때 고친다. exclude와 기본 제외에 걸린 경로와, subDir가 false일 때 감시 대상 바로 아래 폴더
// 안의 항목은 기록에 없다.
func (r *watchTargetRunner) recorded(path string) (isDir, ok bool) {
	if path == r.watchRoot {
		_, ok := r.watchedDirs[path]
		return ok, ok
	}
	e, ok := r.watchedDirs[filepath.Dir(path)][filepath.Base(path)]
	if !ok || e.excluded {
		return false, false
	}
	return e.dir, true
}

// markWatched는 감시 목록에 넣은 폴더 dir을 watchedDirs에 적는다. dir 안의 항목은 addEntry가 적는다.
func (r *watchTargetRunner) markWatched(dir string) {
	if _, ok := r.watchedDirs[dir]; !ok {
		r.watchedDirs[dir] = map[string]entryInfo{}
	}
}

// addEntry는 감시 중인 폴더 안에 있는 항목 path를 그 폴더의 항목으로 적는다. 감시하지 않는 폴더 안의 항목과,
// kqueue가 아닌 운영체제에서 제외에 걸린 항목은 적지 않는다.
func (r *watchTargetRunner) addEntry(path string, isDir bool) {
	entries := r.watchedDirs[filepath.Dir(path)]
	if entries == nil {
		return
	}
	e := entryInfo{dir: isDir, excluded: r.isExcluded(path)}
	if e.excluded && !kqueueOS {
		return
	}
	if kqueueOS {
		if info, err := os.Lstat(path); err == nil {
			e.id, _ = fileIDOf(info)
		}
		if e.id != (fileID{}) {
			r.entryPaths[e.id] = path
		}
	}
	// filepath.Base가 돌려주는 이름은 path의 메모리를 그대로 가리키므로, 전체 경로를 붙잡지 않게 복사한다.
	entries[strings.Clone(filepath.Base(path))] = e
}

// forgetEntry는 사라진 항목 path를 addEntry의 기록에서 지운다.
func (r *watchTargetRunner) forgetEntry(path string) {
	entries := r.watchedDirs[filepath.Dir(path)]
	e, ok := entries[filepath.Base(path)]
	if !ok {
		return
	}
	delete(entries, filepath.Base(path))
	if r.entryPaths[e.id] == path {
		delete(r.entryPaths, e.id)
	}
}
