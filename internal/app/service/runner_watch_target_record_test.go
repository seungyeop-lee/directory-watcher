package service

import (
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

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

// subDir가 false이면 감시 대상 바로 아래 항목만 기록하고, 그 아래 폴더 안의 항목은 기록하지 않는지 확인한다.
func TestRecordWithoutSubDirHoldsOnlyDirectChildren(t *testing.T) {
	f := newRenameFixture(t)
	f.sentinelDirs = []string{""}
	f.write("top.txt")
	f.fill("src/sub", 2)
	f.fill("gone", 2)
	f.startWith(watchTargetOption(domain.WatchOption{FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}}))
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
