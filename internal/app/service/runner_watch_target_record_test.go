package service

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// stop은 runner를 멈추고 감시 루프가 끝날 때까지 기다린다. 그 뒤에는 기록을 읽어도 감시 루프와 겹치지 않는다.
func (f *renameFixture) stop() {
	f.t.Helper()
	var wg sync.WaitGroup
	wg.Add(1)
	f.runner.Stop(&wg)
}

// assertRecorded는 감시 대상 기준 상대 경로 rel이 기록에 있는지와 폴더인지가 want, wantDir와 같은지 확인한다.
func (f *renameFixture) assertRecorded(rel string, want, wantDir bool) {
	f.t.Helper()
	isDir, ok := f.runner.recorded(f.path(rel))
	switch {
	case ok != want:
		f.t.Errorf("recorded(%s) = %v, want %v", rel, ok, want)
	case ok && isDir != wantDir:
		f.t.Errorf("recorded(%s) reports folder %v, want %v", rel, isDir, wantDir)
	}
}

// assertRecordMatchesDisk는 감시 대상 안에 있는 모든 항목이 기록에 같은 종류로 있고, gone의 경로는 기록에
// 없는지 확인한다. exclude와 watchSubDir: false가 기록에서 빼는 항목이 없는 감시 대상에서만 쓴다.
func (f *renameFixture) assertRecordMatchesDisk(gone ...string) {
	f.t.Helper()
	err := filepath.WalkDir(f.target, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(f.target, p)
		if err != nil {
			return err
		}
		f.assertRecorded(filepath.ToSlash(rel), true, d.IsDir())
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	for _, rel := range gone {
		f.assertRecorded(rel, false, false)
	}
}

// fillMovable은 이름을 바꾸거나 옮길 폴더 dir과 그 아래 sub 폴더에 파일을 채운다. Windows는 감시 중인 하위 폴더가
// 있는 폴더의 이름 바꾸기와 옮기기를 거부하므로, Windows에서는 sub 폴더를 만들지 않는다.
func (f *renameFixture) fillMovable(dir string) {
	f.t.Helper()
	f.fill(dir, 3)
	if runtime.GOOS != "windows" {
		f.fill(dir+"/sub", 3)
	}
}

// 감시를 시작할 때와 항목을 만들거나 지우거나 옮긴 뒤에, 기록이 감시 대상 안의 실제 파일과 폴더와 같은지
// 확인한다.
func TestRecordMatchesDiskAfterEachChange(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *renameFixture)
		act   func(f *renameFixture)
		gone  []string
	}{
		{
			name:  "existing at start",
			setup: func(f *renameFixture) { f.fill("src/a/b", 3); f.write("top.txt") },
		},
		{name: "create a file", act: func(f *renameFixture) { f.write("src/new.txt") }},
		{name: "create folders with files", act: func(f *renameFixture) { f.fill("src/n/m", 3) }},
		{
			name:  "remove a file",
			setup: func(f *renameFixture) { f.write("src/a.txt") },
			act:   func(f *renameFixture) { f.removeAll("src/a.txt") },
			gone:  []string{"src/a.txt"},
		},
		{
			name:  "rename a file",
			setup: func(f *renameFixture) { f.write("src/a.txt") },
			act:   func(f *renameFixture) { f.move("src/a.txt", "src/b.txt") },
			gone:  []string{"src/a.txt"},
		},
		{
			name:  "rename a folder",
			setup: func(f *renameFixture) { f.fillMovable("src/d") },
			act:   func(f *renameFixture) { f.move("src/d", "src/e") },
			gone:  []string{"src/d", "src/d/sub", "src/d/sub/f0.txt"},
		},
		{
			name:  "move a folder under another folder",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act:   func(f *renameFixture) { f.move("src/d", "dst/d") },
			gone:  []string{"src/d", "src/d/f0.txt"},
		},
		{
			name:  "move a file in",
			setup: func(f *renameFixture) { f.write("../outside/a.txt") },
			act:   func(f *renameFixture) { f.move("../outside/a.txt", "src/a.txt") },
		},
		{
			name:  "move a folder in",
			setup: func(f *renameFixture) { f.fill("../outside/x/sub", 3) },
			act:   func(f *renameFixture) { f.move("../outside/x", "src/x") },
		},
		{
			name:  "move a file out",
			setup: func(f *renameFixture) { f.write("src/a.txt") },
			act:   func(f *renameFixture) { f.move("src/a.txt", "../outside/a.txt") },
			gone:  []string{"src/a.txt"},
		},
		{
			name:  "move a folder out",
			setup: func(f *renameFixture) { f.fillMovable("src/pkg") },
			act:   func(f *renameFixture) { f.move("src/pkg", "../outside/pkg") },
			gone:  []string{"src/pkg", "src/pkg/sub", "src/pkg/sub/f0.txt"},
		},
		{
			name:  "rm -rf a folder",
			setup: func(f *renameFixture) { f.fill("src/pkg/sub", 3) },
			act:   func(f *renameFixture) { f.removeAll("src/pkg") },
			gone:  []string{"src/pkg", "src/pkg/sub", "src/pkg/sub/f0.txt"},
		},
		{
			name:  "rm -rf and make the folder again",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act: func(f *renameFixture) {
				f.removeAll("src/d")
				f.write("src/d/new.txt")
			},
			gone: []string{"src/d/f0.txt"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRenameFixture(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			f.start(nil)
			if tc.act != nil {
				tc.act(f)
			}
			f.changes()
			f.stop()
			f.assertRecordMatchesDisk(tc.gone...)
		})
	}
}

// exclude와 기본 제외에 걸린 경로는 감시를 시작할 때 있었든 감시 중에 생겼든 기록에 없는지 확인한다.
func TestRecordSkipsExcludedPaths(t *testing.T) {
	f := newRenameFixture(t)
	f.write("dist/old.txt")
	f.write("src/old.tmp")
	f.write("src/.hidden/old.txt")
	f.write("src/keep.txt")
	f.start(domain.ExcludePatterns{"dist/**", "**/*.tmp"})
	f.write("dist/new.txt")
	f.write("src/new.tmp")
	f.write("src/_new.txt")
	f.write("src/added.txt")
	f.changes()
	f.stop()
	for _, rel := range []string{"dist", "dist/old.txt", "dist/new.txt", "src/old.tmp", "src/new.tmp", "src/.hidden", "src/.hidden/old.txt", "src/_new.txt"} {
		f.assertRecorded(rel, false, false)
	}
	f.assertRecorded("src", true, true)
	f.assertRecorded("src/keep.txt", true, false)
	f.assertRecorded("src/added.txt", true, false)
}

// watchSubDir가 false이면 감시 대상 바로 아래 항목만 기록하고, 그 아래 폴더 안의 항목은 기록하지 않는지 확인한다.
func TestRecordWithoutWatchSubDirHoldsOnlyDirectChildren(t *testing.T) {
	f := newRenameFixture(t)
	f.sentinelDirs = []string{""}
	f.write("top.txt")
	f.fill("src/sub", 2)
	f.fill("gone", 2)
	f.startWith(domain.WatchTargetsOption{
		Deliver:         domain.DeliverEach,
		DirEventWait:    100,
		DirEventMaxWait: 1000,
		WatchFileEvent:  domain.WatchAllEvent(),
		WatchDirEvent:   domain.WatchEvent{Create: true, Delete: true},
	})
	f.fill("new", 2)
	f.write("added.txt")
	f.removeAll("gone")
	f.changes()
	f.stop()
	f.assertRecorded("", true, true)
	f.assertRecorded("top.txt", true, false)
	f.assertRecorded("added.txt", true, false)
	f.assertRecorded("src", true, true)
	f.assertRecorded("dst", true, true)
	f.assertRecorded("new", true, true)
	for _, rel := range []string{"src/sub", "src/sub/f0.txt", "new/f0.txt", "gone"} {
		f.assertRecorded(rel, false, false)
	}
}
