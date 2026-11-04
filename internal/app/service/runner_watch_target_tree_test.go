package service

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// treeFixture는 감시 대상 root를 실제 watcher로 감시하는 watchTree를 둔다. 감시 루프 없이 handleEvent가 부르는
// 메서드를 직접 부른다.
type treeFixture struct {
	t    *testing.T
	root string
	tree *watchTree
}

func newTreeFixture(t *testing.T, option domain.WatchOption) *treeFixture {
	return newTreeFixtureAt(t, t.TempDir(), option)
}

// newTreeFixtureAt은 path를 감시 대상으로 하는 treeFixture를 만든다. path는 이 함수를 부르기 전에 만든 TempDir
// 안에 있어야 TempDir 정리보다 먼저 watcher가 닫힌다. Windows는 감시 중인 폴더를 지우지 못한다.
func newTreeFixtureAt(t *testing.T, path string, option domain.WatchOption) *treeFixture {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	f := &treeFixture{t: t, root: path}
	f.tree = newWatchTree(domain.Path(path), option, watcher, stop, newMockLogger())
	t.Cleanup(func() {
		close(stop)
		f.tree.close()
	})
	return f
}

func (f *treeFixture) path(rel string) string {
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

func (f *treeFixture) mkdirAll(rels ...string) {
	f.t.Helper()
	for _, rel := range rels {
		if err := os.MkdirAll(f.path(rel), 0700); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *treeFixture) add() {
	f.t.Helper()
	if err := f.tree.add(); err != nil {
		f.t.Fatal(err)
	}
}

// addCreated는 감시 대상 바로 아래 항목을 handleEvent가 Create를 받았을 때처럼 added로 넣는다.
func (f *treeFixture) addCreated() {
	f.t.Helper()
	entries, err := os.ReadDir(f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, entry := range entries {
		p := filepath.Join(f.root, entry.Name())
		info, err := os.Lstat(p)
		if err != nil {
			f.t.Fatal(err)
		}
		if err := f.tree.added(p, info.IsDir()); err != nil {
			f.t.Fatal(err)
		}
	}
}

// watchList는 watcher의 감시 목록을 감시 대상 root 기준 상대 경로로 정렬해 돌려준다. root는 "."이다.
func (f *treeFixture) watchList() []string {
	f.t.Helper()
	var got []string
	for _, p := range f.tree.watcher.WatchList() {
		rel, err := filepath.Rel(f.tree.root, p)
		if err != nil {
			f.t.Fatal(err)
		}
		got = append(got, filepath.ToSlash(rel))
	}
	slices.Sort(got)
	return got
}

func (f *treeFixture) assertWatchList(want ...string) {
	f.t.Helper()
	if got := f.watchList(); !slices.Equal(got, want) {
		f.t.Errorf("watch list = %v, want %v", got, want)
	}
}

// 감시를 시작할 때 감시 대상 안의 모든 항목이 같은 종류로 기록되는지 확인한다.
func TestWatchTreeRecordsEveryEntryAtStart(t *testing.T) {
	f := newTreeFixture(t, domain.WatchOption{SubDir: true})
	f.mkdirAll("src/a/b", "dst")
	for _, rel := range []string{"top.txt", "src/a/b/f0.txt", "src/a/f1.txt"} {
		writeFile(t, f.path(rel))
	}
	f.add()
	assertRecordMatchesDisk(t, f.tree, f.root)
	f.assertWatchList(".", "dst", "src", "src/a", "src/a/b")
}

// 폴더가 감시를 시작할 때 있었든 감시 중에 생겼든 같은 규칙으로 감시 목록에 들어가는지 확인한다. 제외된 폴더와
// 그 아래 폴더, 폴더를 가리키는 링크는 감시 목록에 들어가지 않는다.
func TestWatchTreeAppliesSameWatchRuleAtStartAndWhenAdded(t *testing.T) {
	cases := []struct {
		name    string
		dirs    []string
		link    string // 감시 대상 밖 폴더를 가리키는 링크의 이름
		exclude domain.ExcludePatterns
		want    []string
	}{
		{name: "plain nested folder", dirs: []string{"plain/sub"}, want: []string{".", "plain", "plain/sub"}},
		{name: "default-excluded folder", dirs: []string{".cache/sub", "plain/_tmp"}, want: []string{".", "plain"}},
		{name: "exclude sibling with same prefix", dirs: []string{"dir1", "dir10"}, exclude: domain.ExcludePatterns{"dir1/**"}, want: []string{".", "dir10"}},
		{name: "exclude subfolder", dirs: []string{"parent/sub/deep"}, exclude: domain.ExcludePatterns{"parent/sub/**"}, want: []string{".", "parent"}},
		{name: "exclude folder at any depth", dirs: []string{"a/node_modules/pkg"}, exclude: domain.ExcludePatterns{"**/node_modules/**"}, want: []string{".", "a"}},
		{name: "symlink to folder", link: "link", want: []string{"."}},
	}
	for _, tc := range cases {
		for _, atStart := range []bool{true, false} {
			name := tc.name + "/added while watching"
			if atStart {
				name = tc.name + "/existing at start"
			}
			t.Run(name, func(t *testing.T) {
				outside := t.TempDir()
				f := newTreeFixture(t, domain.WatchOption{SubDir: true, Exclude: tc.exclude})
				build := func() {
					f.mkdirAll(tc.dirs...)
					if tc.link != "" {
						if err := os.Symlink(outside, f.path(tc.link)); err != nil {
							t.Skipf("symlink not available: %v", err)
						}
					}
				}
				if atStart {
					build()
					f.add()
				} else {
					f.add()
					build()
					f.addCreated()
				}
				f.assertWatchList(tc.want...)
				if tc.link != "" {
					assertRecorded(t, f.tree, f.root, tc.link, true, false)
				}
			})
		}
	}
}

// subDir가 false이면 감시 대상만 감시 목록에 넣고, 감시 대상 바로 아래 항목만 기록하는지 확인한다.
func TestWatchTreeWithoutSubDirWatchesOnlyTarget(t *testing.T) {
	f := newTreeFixture(t, domain.WatchOption{})
	f.mkdirAll("src/sub")
	writeFile(t, f.path("top.txt"))
	f.add()
	f.mkdirAll("new/sub")
	if err := f.tree.added(f.path("new"), true); err != nil {
		t.Fatal(err)
	}
	f.assertWatchList(".")
	assertRecorded(t, f.tree, f.root, "", true, true)
	assertRecorded(t, f.tree, f.root, "top.txt", true, false)
	assertRecorded(t, f.tree, f.root, "src", true, true)
	assertRecorded(t, f.tree, f.root, "new", true, true)
	assertRecorded(t, f.tree, f.root, "src/sub", false, false)
	assertRecorded(t, f.tree, f.root, "new/sub", false, false)
}

// 감시 대상 경로가 폴더를 가리키는 링크이면 링크를 푼 폴더를 감시하고, exclude 패턴도 그 폴더 기준으로 맞추는지
// 확인한다.
func TestWatchTreeLinkedTargetWatchesLinkedFolder(t *testing.T) {
	base := t.TempDir()
	linked, link := filepath.Join(base, "linked"), filepath.Join(base, "link")
	for _, dir := range []string{"sub", "ex"} {
		if err := os.MkdirAll(filepath.Join(linked, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(linked, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	// macOS의 TempDir처럼 상위 경로에 링크가 있어도 모두 푼 경로로 맞춘다.
	resolved, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	f := newTreeFixtureAt(t, link, domain.WatchOption{SubDir: true, Exclude: domain.ExcludePatterns{"ex/**"}})
	f.add()
	if f.tree.root != resolved {
		t.Fatalf("root = %q, want %q", f.tree.root, resolved)
	}
	f.assertWatchList(".", "sub")
	if !f.tree.contains(filepath.Join(resolved, "sub", "a.txt")) || f.tree.contains(filepath.Join(link, "sub", "a.txt")) {
		t.Errorf("contains does not judge paths by the resolved folder %q", resolved)
	}
}

// 사라진 폴더의 감시를 풀면 그 아래 폴더의 감시와 기록도 함께 없어지고, 다른 폴더는 그대로인지 확인한다.
func TestWatchTreeUnwatchTreeDropsFolderAndBelow(t *testing.T) {
	f := newTreeFixture(t, domain.WatchOption{SubDir: true})
	f.mkdirAll("d/sub", "keep")
	writeFile(t, f.path("d/sub/f0.txt"))
	writeFile(t, f.path("top.txt"))
	f.add()
	if !f.tree.unwatchTree(f.path("d")) {
		t.Error("unwatchTree(d) reports d was not a watched folder")
	}
	if f.tree.unwatchTree(f.path("top.txt")) {
		t.Error("unwatchTree(top.txt) reports a file as a watched folder")
	}
	f.assertWatchList(".", "keep")
	for _, rel := range []string{"d", "d/sub", "d/sub/f0.txt", "top.txt"} {
		assertRecorded(t, f.tree, f.root, rel, false, false)
	}
	assertRecorded(t, f.tree, f.root, "keep", true, true)
}

// 감시 목록에 기록 밖의 폴더가 남으면 releaseLeakedWatches가 기록의 폴더만 감시하는 watcher로 갈아 끼우고, 남은
// 감시가 없으면 watcher를 그대로 둔 채 풀지 못한 폴더의 기록만 지우는지 확인한다.
func TestWatchTreeReleaseLeakedWatches(t *testing.T) {
	for _, leaked := range []bool{true, false} {
		name := "no leaked watch"
		if leaked {
			name = "leaked watch"
		}
		t.Run(name, func(t *testing.T) {
			outside := t.TempDir()
			f := newTreeFixture(t, domain.WatchOption{SubDir: true})
			f.mkdirAll("d")
			f.add()
			if leaked {
				// Windows에서 옮겨 나간 폴더의 감시가 남은 상태를 만든다.
				if err := f.tree.watcher.Add(outside); err != nil {
					t.Fatal(err)
				}
			}
			gone := f.path("gone")
			f.tree.unreleasedDirs[gone] = struct{}{}
			events := f.tree.Events()
			if err := f.tree.releaseLeakedWatches(); err != nil {
				t.Fatal(err)
			}
			// 감시 루프처럼 갈아 끼운 예전 watcher의 이벤트를 다 받는다.
			for range f.tree.retireWatcher() {
			}
			f.assertWatchList(".", "d")
			if replaced := f.tree.Events() != events; replaced != leaked {
				t.Errorf("watcher replaced = %v, want %v", replaced, leaked)
			}
			if f.tree.isUnreleasedDir(gone) {
				t.Error("the unreleased folder is still recorded")
			}
		})
	}
}
