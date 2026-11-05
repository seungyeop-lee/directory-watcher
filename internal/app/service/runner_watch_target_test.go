package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

func deletionRunner(t *testing.T, root string, option domain.WatchTargetsOption) *watchTargetRunner {
	t.Helper()
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: option}, newMockLogger(), process.CommandMode{})
	t.Cleanup(func() { close(r.stop); r.tree.close() })
	if err := r.tree.add(); err != nil {
		t.Fatal(err)
	}
	r.dirBatcher.root = r.tree.root
	return r
}

func sendDeletionEvent(t *testing.T, r *watchTargetRunner, op fsnotify.Op, rel string) {
	t.Helper()
	if err := r.handleEvent(fsnotify.Event{Name: filepath.Join(r.path.String(), filepath.FromSlash(rel)), Op: op}); err != nil {
		t.Fatal(err)
	}
}

func flushDeletionRuns(t *testing.T, r *watchTargetRunner) []domain.Event {
	t.Helper()
	if r.dirBatcher.batch != nil {
		if err := r.enqueue(r.dirBatcher.flush()); err != nil {
			t.Fatal(err)
		}
	}
	if r.hookBatcher.batch != nil {
		if err := r.dispatcher.enqueue(r.hookBatcher.flush()); err != nil {
			t.Fatal(err)
		}
	}
	runs := r.dispatcher.queue.runs
	r.dispatcher.queue.runs = nil
	return runs
}

func TestDeletedFolderLifecycleAcrossBatches(t *testing.T) {
	setDirRemoveRepeat(t, true)
	for _, separated := range []bool{false, true} {
		for _, deliver := range []domain.Deliver{domain.DeliverLast, domain.DeliverLastPerPath, domain.DeliverEach} {
			for _, filter := range []string{"all", "files", "folders"} {
				t.Run(fmt.Sprintf("separated=%v/deliver=%v/%s", separated, deliver, filter), func(t *testing.T) {
					root := t.TempDir()
					mkdir(t, filepath.Join(root, "pkg.v2"))
					writeFile(t, filepath.Join(root, "pkg.v2", "child.txt"))
					option := hookBatchOption(1000, 10000, deliver)
					option.DirBatch = domain.DirBatchOption{}
					if separated {
						option.DirBatch = domain.DirBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: 1000}
					}
					if filter == "files" {
						option.Watch.DirEvent = domain.WatchEvent{}
					}
					if filter == "folders" {
						option.Watch.FileEvent = domain.WatchEvent{}
					}
					r := deletionRunner(t, root, option)
					removeAll(t, filepath.Join(root, "pkg.v2"))
					sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2/child.txt")
					if separated {
						if err := r.enqueue(r.dirBatcher.flush()); err != nil {
							t.Fatal(err)
						}
					}
					sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
					if separated {
						if err := r.enqueue(r.dirBatcher.flush()); err != nil {
							t.Fatal(err)
						}
					}
					sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
					runs := flushDeletionRuns(t, r)
					got, want := []string{}, []string{}
					if (deliver == domain.DeliverEach && filter != "folders") || (deliver == domain.DeliverLast && filter == "files") {
						want = append(want, "D pkg.v2/child.txt")
					}
					if filter != "files" {
						want = append(want, "D pkg.v2/")
					}
					for _, run := range runs {
						got = append(got, describeRun(t, root, run))
						built, err := (command.SingleCmd{Shell: command.DefaultShell(), Cmd: "{{.IsDir}} [{{.ExtName}}]"}).Build(context.Background(), domain.Path(root), &run)
						if err != nil {
							t.Fatal(err)
						}
						wantFilled, ext := "true []", ""
						if !run.IsDir {
							wantFilled, ext = "false [.txt]", ".txt"
						}
						if built[0].Filled != wantFilled || !slices.Contains(built[0].Cmd.Env, fmt.Sprintf("DW_IS_DIR=%v", run.IsDir)) || !slices.Contains(built[0].Cmd.Env, "DW_EXT_NAME="+ext) {
							t.Errorf("command metadata = %q, want %q with matching DW_IS_DIR/DW_EXT_NAME", built[0].Filled, wantFilled)
						}
					}
					if !slices.Equal(got, want) {
						t.Fatalf("runs = %v, want %v", got, want)
					}
					assertRecorded(t, r.tree, root, "pkg.v2", false, false)
					assertRecorded(t, r.tree, root, "pkg.v2/child.txt", false, false)
					for _, path := range r.tree.watcher.WatchList() {
						if domain.Path(filepath.Join(root, "pkg.v2")).Contains(domain.Path(path)) {
							t.Errorf("deleted folder remains watched: %s", path)
						}
					}
					sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
					sendDeletionEvent(t, r, fsnotify.Write, "pkg.v2")
					if late := flushDeletionRuns(t, r); len(late) != 0 {
						t.Fatalf("late notifications within removedDirTTL after hook flush = %v", late)
					}
				})
			}
		}
	}
}

func TestDeletedFolderRecreationStartsNewLifecycle(t *testing.T) {
	setDirRemoveRepeat(t, true)
	for _, subDir := range []bool{false, true} {
		for _, replacement := range []string{"file", "folder", "file without create"} {
			t.Run(fmt.Sprintf("subDir=%v/%s", subDir, replacement), func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "pkg.v2")
				mkdir(t, path)
				option := hookBatchOption(0, 0, domain.DeliverEach)
				option.DirBatch = domain.DirBatchOption{}
				option.Watch.SubDir = subDir
				r := deletionRunner(t, root, option)
				removeAll(t, path)
				sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
				sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
				first := flushDeletionRuns(t, r)
				if len(first) != 1 || !first[0].IsDir {
					t.Fatalf("first deletion = %v, want one folder deletion", first)
				}
				if replacement == "folder" {
					mkdir(t, path)
				} else {
					writeFile(t, path)
				}
				op := fsnotify.Create
				if replacement == "file without create" {
					op = fsnotify.Write
				}
				sendDeletionEvent(t, r, op, "pkg.v2")
				created := flushDeletionRuns(t, r)
				if len(created) != 1 || created[0].IsDir != (replacement == "folder") || fsnotify.Op(created[0].Operation) != op {
					t.Fatalf("replacement = %v, want %s folder=%v", created, op, replacement == "folder")
				}
				removeAll(t, path)
				sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
				deleted := flushDeletionRuns(t, r)
				if len(deleted) != 1 || deleted[0].IsDir != (replacement == "folder") || fsnotify.Op(deleted[0].Operation) != fsnotify.Remove {
					t.Fatalf("replacement deletion = %v, want folder=%v", deleted, replacement == "folder")
				}
			})
		}
	}
}

// kqueue(macOS)에서 감시 중인 폴더를 옮기거나 지운 직후 같은 경로에 폴더를 다시 만들면, fsnotify는 새 폴더의 Create를
// 보내지 않거나 부모 폴더가 다시 바뀔 때에야 보낸다. 어느 쪽이든 hook이 옛 폴더의 D 다음에 새 폴더의 C를 한 번씩 받고,
// 새 폴더를 감시하는지 확인한다.
func TestFolderRecreatedBeforeItsRemoveArrives(t *testing.T) {
	if !kqueueOS {
		t.Skip("only fsnotify's kqueue backend drops the Create of a folder recreated at the same path")
	}
	for _, op := range []fsnotify.Op{fsnotify.Rename, fsnotify.Remove} {
		for _, lateCreate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/lateCreate=%v", op, lateCreate), func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "d")
				mkdir(t, path)
				option := hookBatchOption(0, 0, domain.DeliverEach)
				option.DirBatch = domain.DirBatchOption{}
				r := deletionRunner(t, root, option)
				if op == fsnotify.Rename {
					if err := os.Rename(path, filepath.Join(t.TempDir(), "d")); err != nil {
						t.Fatal(err)
					}
				} else {
					removeAll(t, path)
				}
				mkdir(t, path)
				sendDeletionEvent(t, r, op, "d")
				if lateCreate {
					sendDeletionEvent(t, r, fsnotify.Create, "d")
				}
				var got []string
				for _, run := range flushDeletionRuns(t, r) {
					got = append(got, describeRun(t, root, run))
				}
				if want := []string{"D d/", "C d/"}; !slices.Equal(got, want) {
					t.Fatalf("runs = %v, want %v", got, want)
				}
				assertRecorded(t, r.tree, root, "d", true, true)
				if !slices.ContainsFunc(r.tree.watcher.WatchList(), func(p string) bool { return filepath.Clean(p) == path }) {
					t.Errorf("recreated folder is not watched (%v)", r.tree.watcher.WatchList())
				}
			})
		}
	}
}

// 이름이 매번 다른 폴더를 지워도, 지운 폴더의 기록은 중복 REMOVE가 올 수 있는 운영체제에서 removedDirTTL 안에 지운
// 폴더만 남고 그 밖의 운영체제에서는 생기지 않는지 확인한다. 어느 쪽이든 폴더마다 폴더 삭제 하나만 hook에 간다.
func TestRemovedFolderRecordsExpire(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		t.Run(fmt.Sprintf("repeat=%v", repeat), func(t *testing.T) {
			setDirRemoveRepeat(t, repeat)
			root := t.TempDir()
			const folders = 50
			for i := range folders {
				mkdir(t, filepath.Join(root, fmt.Sprintf("tmp-%04d", i)))
			}
			option := hookBatchOption(0, 0, domain.DeliverEach)
			option.DirBatch = domain.DirBatchOption{}
			r := deletionRunner(t, root, option)
			now := time.Now()
			r.removedDirs.now = func() time.Time { return now }
			remove := func(i int) {
				name := fmt.Sprintf("tmp-%04d", i)
				removeAll(t, filepath.Join(root, name))
				sendDeletionEvent(t, r, fsnotify.Remove, name)
				if repeat {
					sendDeletionEvent(t, r, fsnotify.Remove, name)
				}
			}
			for i := range folders - 1 {
				remove(i)
			}
			want := 0
			if repeat {
				want = folders - 1
			}
			if got := len(r.removedDirs.at); got != want {
				t.Fatalf("records within removedDirTTL = %d, want %d", got, want)
			}
			now = now.Add(removedDirTTL + 100*time.Millisecond)
			remove(folders - 1)
			if repeat {
				want = 1
			}
			if got := len(r.removedDirs.at); got != want {
				t.Fatalf("records after removedDirTTL = %d, want %d", got, want)
			}
			runs := flushDeletionRuns(t, r)
			if len(runs) != folders || slices.ContainsFunc(runs, func(run domain.Event) bool { return !run.IsDir }) {
				t.Fatalf("runs = %v, want one folder deletion per folder", runs)
			}
		})
	}
}

// 폴더를 지운 경로에 파일이나 폴더가 다시 생겼다가 지워지면, 지운 폴더의 중복 알림이 오는 운영체제인지와 관계없이
// 새 항목의 변경과 삭제가 앞선 폴더 삭제에 흡수되지 않는지 확인한다.
func TestDeletionRecreationWithinDirBatch(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		for _, replacement := range []string{"file", "folder", "file without create"} {
			for _, deliver := range []domain.Deliver{domain.DeliverLast, domain.DeliverLastPerPath, domain.DeliverEach} {
				t.Run(fmt.Sprintf("repeat=%v/%s/deliver=%v", repeat, replacement, deliver), func(t *testing.T) {
					setDirRemoveRepeat(t, repeat)
					root := t.TempDir()
					path := filepath.Join(root, "pkg.v2")
					mkdir(t, path)
					r := deletionRunner(t, root, hookBatchOption(1000, 10000, deliver))
					removeAll(t, path)
					sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
					suffix := ""
					if replacement == "folder" {
						mkdir(t, path)
						suffix = "/"
					} else {
						writeFile(t, path)
					}
					op, symbol := fsnotify.Create, "C"
					if replacement == "file without create" {
						op, symbol = fsnotify.Write, "U"
					}
					sendDeletionEvent(t, r, op, "pkg.v2")
					removeAll(t, path)
					sendDeletionEvent(t, r, fsnotify.Remove, "pkg.v2")
					got, want := []string{}, []string{"D pkg.v2" + suffix}
					if deliver == domain.DeliverEach {
						want = []string{"D pkg.v2/", symbol + " pkg.v2" + suffix, "D pkg.v2" + suffix}
					}
					for _, run := range flushDeletionRuns(t, r) {
						got = append(got, describeRun(t, root, run))
					}
					if !slices.Equal(got, want) {
						t.Fatalf("runs = %v, want %v", got, want)
					}
				})
			}
		}
	}
}

// 생성 이벤트를 빠뜨린 파일은 처음 수정 이벤트에서 기록에 들어가, 다음에 기다리는 동안의 수정은 수정으로 나가는지 확인한다.
func TestFileWithoutCreateEventIsRecordedOnWrite(t *testing.T) {
	f := startControlledRunner(t, t.TempDir(), hookBatchOption(50, 1000, domain.DeliverLast), nil)
	writeFile(f.t, f.path("f.txt"))
	f.send(fsnotify.Write, "f.txt")
	if got, want := f.collect(300*time.Millisecond), []string{"C f.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs after the first write = %v, want %v", got, want)
	}
	f.send(fsnotify.Write, "f.txt")
	if got, want := f.collect(300*time.Millisecond), []string{"U f.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs after the second write = %v, want %v", got, want)
	}
}

// hook이 도는 동안 기다리는 시간이 여러 번 끝나면, 그때마다 만든 실행이 대기열에 따로 쌓여 하나로 합쳐지지 않는지 확인한다.
func TestRunsFromSeparateWaitsAreNotMerged(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f := startControlledRunner(t, t.TempDir(), hookBatchOption(20, 1000, domain.DeliverLast), func(domain.Event) {
		once.Do(func() { close(entered); <-release })
	})
	writeFile(f.t, f.path("a.txt"))
	f.send(fsnotify.Create, "a.txt")
	<-entered
	for _, name := range []string{"b.txt", "c.txt"} {
		writeFile(f.t, f.path(name))
		f.send(fsnotify.Create, name)
		time.Sleep(100 * time.Millisecond)
	}
	close(release)
	if got, want := f.collect(300*time.Millisecond), []string{"C a.txt", "C b.txt", "C c.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}

// 폴더가 안의 내용과 함께 생기거나 사라지면 hook이 맨 위 폴더의 이벤트 하나만 받는지 확인한다. 폴더 안의
// 파일 300개를 옮겨 오거나 지워도 그 파일들의 이벤트는 대기열에 들어가지 않는다.
func TestFolderChangeNotifiesTopFolderOnce(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *renameFixture)
		act   func(f *renameFixture)
		want  []string
	}{
		{name: "mkdir", act: func(f *renameFixture) { f.mkdirAll("src/pkg") }, want: []string{"C src/pkg/"}},
		{name: "mkdir -p", act: func(f *renameFixture) { f.mkdirAll("src/a/b/c") }, want: []string{"C src/a/"}},
		{
			name:  "move in a folder with files",
			setup: func(f *renameFixture) { f.fill("../outside/x", 3); f.write("../outside/x/sub/g.txt") },
			act:   func(f *renameFixture) { f.move("../outside/x", "src/x") },
			want:  []string{"C src/x/"},
		},
		{
			name:  "move in a folder with 300 files",
			setup: func(f *renameFixture) { f.fill("../outside/x", 300) },
			act:   func(f *renameFixture) { f.move("../outside/x", "src/x") },
			want:  []string{"C src/x/"},
		},
		{
			name: "mkdir and write a file in it",
			act: func(f *renameFixture) {
				f.mkdirAll("src/n")
				f.write("src/n/a.txt")
			},
			want: []string{"C src/n/"},
		},
		{
			name:  "rm -rf a folder with 300 files",
			setup: func(f *renameFixture) { f.fill("src/pkg", 300) },
			act:   func(f *renameFixture) { f.removeAll("src/pkg") },
			want:  []string{"D src/pkg/"},
		},
		{
			name: "rm -rf nested folders",
			setup: func(f *renameFixture) {
				f.fill("src/pkg", 5)
				f.fill("src/pkg/sub", 5)
				f.write("src/pkg/sub/deep/g.txt")
			},
			act:  func(f *renameFixture) { f.removeAll("src/pkg") },
			want: []string{"D src/pkg/"},
		},
		{
			name:  "move out of the target",
			setup: func(f *renameFixture) { f.fill("src/pkg", 3) },
			act:   func(f *renameFixture) { f.move("src/pkg", "../outside/pkg") },
			want:  []string{"D src/pkg/"},
		},
		{
			name:  "rename",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act:   func(f *renameFixture) { f.move("src/d", "src/e") },
			want:  []string{"D src/d/", "C src/e/"},
		},
		{
			name:  "move under another folder",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act:   func(f *renameFixture) { f.move("src/d", "dst/d") },
			want:  []string{"D src/d/", "C dst/d/"},
		},
		{
			name:  "rm -rf and mkdir again",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act: func(f *renameFixture) {
				f.removeAll("src/d")
				f.mkdirAll("src/d")
			},
			want: []string{"D src/d/", "C src/d/"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRenameFixture(t)
			f.holdDirBatch = true
			if tc.setup != nil {
				tc.setup(f)
			}
			f.start(nil)
			tc.act(f)
			if got := f.changes(); !slices.Equal(got, tc.want) {
				t.Fatalf("hook events = %v, want %v", got, tc.want)
			}
		})
	}
}

// dirEvent가 none이면 폴더 이벤트도, 폴더 이벤트에 흡수된 파일 이벤트도 hook에 가지 않는지 확인한다.
func TestNoFolderEventStillAbsorbsFileEvents(t *testing.T) {
	f := newRenameFixture(t)
	f.holdDirBatch = true
	f.fill("src/pkg", 5)
	f.fill("../outside/x", 3)
	f.startWith(watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent()}))
	f.mkdirAll("src/a/b")
	f.write("src/a/b/c.txt")
	f.move("../outside/x", "src/x")
	f.removeAll("src/pkg")
	if got := f.changes(); len(got) != 0 {
		t.Fatalf("hook events = %v, want none", got)
	}
	f.write("src/a/b/later.txt")
	if got := f.changes(); !slices.Contains(got, "C src/a/b/later.txt") {
		t.Fatalf("creation in the new folder did not reach the hook (got %v)", got)
	}
}

// subDir가 false이면 감시하지 않는 대상 바로 아래 폴더도 만들거나 지우면 폴더 이벤트로 알리는지 확인한다.
func TestFolderEventWithoutSubDir(t *testing.T) {
	f := newRenameFixture(t)
	f.sentinelDirs = []string{""}
	f.fill("src", 3)
	f.startWith(watchTargetOption(domain.WatchOption{FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}}))
	f.removeAll("src")
	if got, want := f.changes(), []string{"D src/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events after removing an existing folder = %v, want %v", got, want)
	}
	f.mkdirAll("new")
	f.fill("new", 3)
	if got, want := f.changes(), []string{"C new/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events after making a folder = %v, want %v", got, want)
	}
	f.removeAll("new")
	if got, want := f.changes(), []string{"D new/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events after removing a new folder = %v, want %v", got, want)
	}
}

// 폴더를 만들고 곧바로 그 안에 파일을 써도 hook이 폴더 C를 받고, 그 뒤 그 폴더 안에 만든 파일의 이벤트도 받는지
// 확인한다. macOS에서는 fsnotify가 새 폴더의 감시 등록을 덮어써 그 폴더 안의 이벤트를 놓치기도 했다. 덮어쓴
// 등록은 다음 이벤트를 받을 때 되돌리므로, 폴더 C를 받기 전에는 다른 이벤트를 만들지 않는다.
func TestFolderCreatedWithFileIsWatchedAfterFolderEvent(t *testing.T) {
	f := newRenameFixture(t)
	f.start(nil)
	f.mkdirAll("src/n")
	f.write("src/n/a.txt")
	if got, want := f.next(), "C src/n/"; got != want {
		t.Fatalf("hook event = %q, want %q", got, want)
	}
	f.write("src/n/b.txt")
	if got := f.changes(); !slices.Contains(got, "C src/n/b.txt") {
		t.Fatalf("creation in the new folder did not reach the hook (got %v)", got)
	}
}

// 폴더 이벤트를 모으는 동안 쌓인 이벤트로 만든 실행이 대기열 용량보다 많으면, 감시 루프는 대기열에 자리가 나기를
// 기다리지 않고 dispatch queue full로 멈춘다. 폴더 안의 파일만 지우면 폴더 이벤트가 없어 파일 이벤트가 모두 실행이 된다.
func TestBatchLargerThanQueueOverflows(t *testing.T) {
	const queueSize, files = 50, 150
	f := newRenameFixture(t)
	f.fill("src", files)
	release := make(chan struct{})
	option := watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}})
	option.Queue.Size = queueSize
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(f.target), Option: option}, newMockLogger(), process.CommandMode{})
	f.changed, f.result = startRunner(t, r, func(domain.Event) { <-release })
	t.Cleanup(func() { close(release) })
	for i := 0; i < files; i++ {
		if err := os.Remove(f.path(fmt.Sprintf("src/f%d.txt", i))); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-f.result:
		if !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full (capacity 50)") {
			t.Fatalf("run stopped with %v, want dispatch queue full (capacity 50)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch loop waited for room in the queue instead of stopping")
	}
}

// 폴더 이벤트는 exclude에 맞는 폴더에는 오지 않는다. dist/**는 dist 폴더 자신도 빼고, dist/*는 dist 안의 항목만 뺀다.
func TestExcludedFolderSendsNoFolderEvent(t *testing.T) {
	f := newRenameFixture(t)
	f.start(domain.ExcludePatterns{"src/dist/**", "dst/out/*"})
	f.mkdirAll("src/dist/sub")
	f.mkdirAll("dst/out/sub")
	got := f.changes()
	if slices.ContainsFunc(got, func(ev string) bool { return strings.Contains(ev, "src/dist") }) {
		t.Errorf("excluded folder src/dist reached the hook (got %v)", got)
	}
	if want := []string{"C dst/out/"}; !slices.Equal(got, want) {
		t.Errorf("hook events = %v, want %v", got, want)
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

func TestDefaultExcludeChecksCreatedFileName(t *testing.T) {
	tests := []struct {
		name      string
		watchPath func(t *testing.T) string
	}{
		{
			name:      "absolute path",
			watchPath: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "relative path starting with ..",
			watchPath: func(t *testing.T) string {
				base := t.TempDir()
				work, src := filepath.Join(base, "work"), filepath.Join(base, "src")
				for _, dir := range []string{work, src} {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				chdir(t, work)
				return filepath.Join("..", "src")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.watchPath(t)
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger(), process.CommandMode{})
			changed, _ := startRunner(t, r, nil)

			// 제외된 이벤트가 오지 않았음을 시간 초과만으로 판단하지 않도록,
			// 제외 대상을 먼저 만들고 일반 파일의 이벤트가 도착할 때까지 받은 경로를 확인한다.
			for _, name := range []string{".hidden.txt", "_draft.txt", "plain.txt"} {
				if err := os.WriteFile(filepath.Join(path, name), []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for {
				got := awaitHookEvent(t, changed, nil)
				switch got.Path.Name() {
				case "plain.txt":
					return
				case ".hidden.txt", "_draft.txt":
					t.Fatalf("create of default-excluded file ran the hook: %s", got.Path)
				}
			}
		})
	}
}

func TestDefaultExcludeChecksModifiedAndRemovedName(t *testing.T) {
	tests := []struct {
		name      string
		watchPath func(t *testing.T) string
	}{
		{
			name:      "plain watch path",
			watchPath: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "watch path named with a dot",
			watchPath: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), ".watched")
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.watchPath(t)
			for _, name := range []string{".env", "_draft.txt", "plain.txt"} {
				if err := os.WriteFile(filepath.Join(path, name), []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(path, ".cache"), 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent()})}, newMockLogger(), process.CommandMode{})
			changed, _ := startRunner(t, r, nil)

			// 제외된 이벤트가 오지 않았음을 시간 초과만으로 판단하지 않도록, 제외 대상을 먼저 바꾸고
			// 일반 파일의 수정과 삭제가 hook에 도착할 때까지 받은 이벤트를 확인한다.
			for _, name := range []string{".env", "_draft.txt"} {
				appendFile(t, filepath.Join(path, name))
			}
			for _, name := range []string{".env", "_draft.txt", ".cache"} {
				if err := os.RemoveAll(filepath.Join(path, name)); err != nil {
					t.Fatal(err)
				}
			}
			plain := filepath.Join(path, "plain.txt")
			appendFile(t, plain)
			waitPlainEvent(t, changed, fsnotify.Write)
			if err := os.Remove(plain); err != nil {
				t.Fatal(err)
			}
			waitPlainEvent(t, changed, fsnotify.Remove)
		})
	}
}

// 빈 경로나 감시 대상 밖 경로의 이벤트는 hook을 실행하지 않고, 대상 밖 폴더를 감시 목록에 넣지도 않는지 확인한다.
func TestEventOutsideWatchTargetIsDropped(t *testing.T) {
	target, outside := t.TempDir(), t.TempDir()
	outsideDir := filepath.Join(outside, "dir")
	if err := os.Mkdir(outsideDir, 0700); err != nil {
		t.Fatal(err)
	}
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(target), Option: watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchEvent{Create: true, Update: true, Delete: true}})}, newMockLogger(), process.CommandMode{})
	events, _ := controlledSource(r)
	changed, result := startRunner(t, r, nil)

	for _, ev := range []fsnotify.Event{
		{Name: "", Op: fsnotify.Write},
		{Name: "", Op: fsnotify.Create},
		{Name: outsideDir, Op: fsnotify.Create},
		{Name: filepath.Join(outside, "a.txt"), Op: fsnotify.Write},
		{Name: filepath.Join(outside, "b.txt"), Op: fsnotify.Remove},
		{Name: filepath.Join("..", "outside", "a.txt"), Op: fsnotify.Write},
	} {
		events <- ev
	}
	// 감시 루프와 hook은 이벤트를 받은 순서대로 처리하므로, 뒤이어 넣은 대상 안 이벤트의 hook이
	// 실행됐다면 앞서 넣은 이벤트의 처리도 끝났다.
	inside := filepath.Join(target, "in.txt")
	events <- fsnotify.Event{Name: inside, Op: fsnotify.Create}
	got := awaitHookEvent(t, changed, result)
	if got.Path.String() != inside {
		t.Fatalf("event outside the watch target ran the hook: %q", got.Path)
	}
	if got := r.tree.watcher.WatchList(); len(got) != 1 || got[0] != target {
		t.Fatalf("watch list changed: %v", got)
	}
}

// exclude에 맞는 폴더와 그 아래 폴더는 감시 시작 때 있었든 감시 중에 생겼든 감시 목록에 들어가지 않는지 확인한다.
func TestExcludedFolderStaysOutOfWatchList(t *testing.T) {
	exclude := domain.ExcludePatterns{"dist/**", "**/node_modules/**"}
	excludedDirs := []string{"dist", "dist/sub", "app/node_modules", "app/node_modules/pkg"}
	watchedDirs := []string{"keep", "keep/sub", "app"}
	for _, atStart := range []bool{true, false} {
		name := "created while watching"
		if atStart {
			name = "existing at start"
		}
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			makeDirs := func() {
				for _, dir := range []string{"dist/sub", "app/node_modules/pkg", "keep/sub"} {
					if err := os.MkdirAll(filepath.Join(target, dir), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			if atStart {
				makeDirs()
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(target), Option: watchTargetOption(domain.WatchOption{SubDir: true, Exclude: exclude, FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger(), process.CommandMode{})
			changed, result := startRunner(t, r, nil)
			if !atStart {
				makeDirs()
			}
			// 감시 루프는 이벤트를 받은 순서대로 처리하므로, 뒤이어 만든 파일의 hook이 실행됐다면 앞서 생긴 폴더의
			// 감시 목록 반영도 끝났다. 가장 깊은 감시 폴더 안의 파일 hook까지 실행되면 감시 목록 반영이 모두 끝났다.
			for _, sentinel := range []string{filepath.Join(target, "zz-registered.txt"), filepath.Join(target, "keep", "sub", "zz-registered.txt")} {
				if err := os.WriteFile(sentinel, []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
				awaitHookPath(t, changed, result, sentinel)
			}

			watchList := map[string]bool{}
			for _, p := range r.tree.watcher.WatchList() {
				watchList[p] = true
			}
			for _, dir := range excludedDirs {
				if watchList[filepath.Join(target, dir)] {
					t.Errorf("excluded folder %s is in the watch list", dir)
				}
			}
			for _, dir := range watchedDirs {
				if !watchList[filepath.Join(target, dir)] {
					t.Errorf("folder %s is not in the watch list", dir)
				}
			}
		})
	}
}

// exclude에 맞는 파일은 생성, 수정, 삭제 모두 hook을 실행하지 않고, 맞지 않는 파일은 hook을 실행하는지 확인한다.
func TestExcludePatternDropsFileEvents(t *testing.T) {
	cases := []struct {
		name     string
		exclude  domain.ExcludePatterns
		excluded []string
		watched  []string
	}{
		{name: "file name", exclude: domain.ExcludePatterns{"out.txt"}, excluded: []string{"out.txt"}, watched: []string{"layout.txt", "sub/out.txt"}},
		{name: "extension", exclude: domain.ExcludePatterns{"**/*.log"}, excluded: []string{"a.log", "sub/b.log"}, watched: []string{"a.txt"}},
		{name: "pattern with a sub path", exclude: domain.ExcludePatterns{"build/**/*.o"}, excluded: []string{"build/x.o", "build/sub/y.o"}, watched: []string{"build/x.c", "src/x.o"}},
		{name: "negation", exclude: domain.ExcludePatterns{"**/*.log", "!important.log"}, excluded: []string{"a.log"}, watched: []string{"important.log"}},
		{name: "negation inside a watched folder", exclude: domain.ExcludePatterns{"dist/*", "!dist/keep.txt"}, excluded: []string{"dist/a.txt"}, watched: []string{"dist/keep.txt"}},
		{name: "negation inside an excluded folder", exclude: domain.ExcludePatterns{"dist/**", "!dist/keep.txt"}, excluded: []string{"dist/a.txt", "dist/keep.txt"}},
	}
	ops := []fsnotify.Op{fsnotify.Create, fsnotify.Write, fsnotify.Remove}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := t.TempDir()
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(target), Option: watchTargetOption(domain.WatchOption{SubDir: true, Exclude: tc.exclude, FileEvent: domain.WatchAllEvent()})}, newMockLogger(), process.CommandMode{})
			events, _ := controlledSource(r)
			changed, result := startRunner(t, r, nil)

			type fileChange struct {
				rel string
				op  fsnotify.Op
			}
			var want []fileChange
			for _, rel := range append(append([]string{}, tc.excluded...), tc.watched...) {
				for _, op := range ops {
					events <- fsnotify.Event{Name: filepath.Join(target, filepath.FromSlash(rel)), Op: op}
				}
			}
			for _, rel := range tc.watched {
				for _, op := range ops {
					want = append(want, fileChange{rel, op})
				}
			}
			// 감시 루프와 hook은 이벤트를 받은 순서대로 처리하므로, 뒤이어 넣은 이벤트의 hook이 실행됐다면
			// 앞서 넣은 이벤트의 처리도 끝났다.
			sentinel := filepath.Join(target, "zz-sentinel.txt")
			events <- fsnotify.Event{Name: sentinel, Op: fsnotify.Create}
			var got []fileChange
			for {
				event := awaitHookEvent(t, changed, result)
				if event.Path.String() == sentinel {
					break
				}
				rel, _ := filepath.Rel(target, event.Path.String())
				got = append(got, fileChange{filepath.ToSlash(rel), fsnotify.Op(event.Operation)})
			}
			if !slices.Equal(got, want) {
				t.Fatalf("hook ran for %v, want %v", got, want)
			}
		})
	}
}

// hook이 감시 대상 안의 out.txt에 써도 exclude에 out.txt를 적었으면 그 쓰기로 hook이 다시 실행되지 않고,
// hook 실행 중에 바뀐 다른 파일은 hook이 끝난 뒤 처리되는지 확인한다.
func TestHookOutputInExcludedPathDoesNotRerunHook(t *testing.T) {
	target := t.TempDir()
	out, a, b, c := filepath.Join(target, "out.txt"), filepath.Join(target, "a.txt"), filepath.Join(target, "b.txt"), filepath.Join(target, "c.txt")
	logger := &eventLogger{lines: make(chan string, 64)}
	release := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var runs []string // hook이 시작하고 끝난 순서
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(target), Option: watchTargetOption(domain.WatchOption{SubDir: true, Exclude: domain.ExcludePatterns{"out.txt"}, FileEvent: domain.WatchAllEvent()})}, logger, process.CommandMode{})
	changed, result := startRunner(t, r, func(event domain.Event) {
		mu.Lock()
		runs = append(runs, "start "+event.Path.Name())
		mu.Unlock()
		f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			_, err = f.WriteString(event.Path.Name() + "\n")
			f.Close()
		}
		if err != nil {
			t.Error(err)
		}
		once.Do(func() { <-release })
		mu.Lock()
		runs = append(runs, "end "+event.Path.Name())
		mu.Unlock()
	})

	if err := os.WriteFile(a, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	// 첫 hook이 끝나지 않은 동안 b.txt를 바꾸고, 감시 루프가 b.txt의 이벤트를 받은 뒤에 hook을 끝낸다.
	logger.await(t, "out.txt")
	if err := os.WriteFile(b, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	logger.await(t, "b.txt")
	close(release)
	seen := awaitHookPath(t, changed, result, b)
	if err := os.WriteFile(c, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	seen = append(seen, awaitHookPath(t, changed, result, c)...)

	for _, p := range seen {
		if p == out {
			t.Fatalf("hook output in out.txt ran the hook (hook paths %v)", seen)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	firstEnd, bStart := slices.Index(runs, "end a.txt"), slices.Index(runs, "start b.txt")
	if firstEnd < 0 || bStart < firstEnd {
		t.Fatalf("change of b.txt was not handled after the running hook ended: %v", runs)
	}
}

// 파일의 이름을 바꾸거나 옮기면 hook이 옛 경로의 삭제(D)를 먼저, 감시 대상 안의 새 경로의 생성(C)을 그다음에
// 받는지 확인한다.
func TestRenamedOrMovedFileNotifiesOldPathAsDelete(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     []string
	}{
		{name: "rename in the same folder", from: "src/a.txt", to: "src/b.txt", want: []string{"D src/a.txt", "C src/b.txt"}},
		{name: "move to another folder in the target", from: "src/a.txt", to: "dst/a.txt", want: []string{"D src/a.txt", "C dst/a.txt"}},
		{name: "move out of the target", from: "src/a.txt", to: "../outside/a.txt", want: []string{"D src/a.txt"}},
		{name: "move into the target", from: "../outside/a.txt", to: "src/a.txt", want: []string{"C src/a.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRenameFixture(t)
			f.write(tc.from)
			f.start(nil)
			f.move(tc.from, tc.to)
			if got := f.changes(); !slices.Equal(got, tc.want) {
				t.Fatalf("hook events = %v, want %v", got, tc.want)
			}
		})
	}
}

// 폴더의 이름을 바꾸거나 옮기면 hook이 옛 경로의 폴더 삭제(D)를 한 번 받고 그다음에 새 경로의 폴더 생성(C)을 받으며,
// 옮긴 폴더 안의 변경은 새 경로로만 받는지 확인한다. 감시 대상 밖으로 옮긴 폴더는 그 아래 폴더까지 감시가 풀려 옮겨
// 간 곳의 변경으로 hook이 실행되지 않는다.
func TestRenamedOrMovedFolderNotifiesOldPathAsDelete(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     []string
	}{
		{name: "rename in the same folder", from: "src/d", to: "src/e", want: []string{"D src/d/", "C src/e/"}},
		{name: "move to another folder in the target", from: "src/d", to: "dst/d", want: []string{"D src/d/", "C dst/d/"}},
		{name: "move out of the target", from: "src/d", to: "../outside/d", want: []string{"D src/d/"}},
		{name: "move into the target", from: "../outside/d", to: "src/d", want: []string{"C src/d/"}},
	}
	for _, nested := range []bool{false, true} {
		for _, tc := range cases {
			name := tc.name
			if nested {
				name += " with a subfolder"
			}
			t.Run(name, func(t *testing.T) {
				f := newRenameFixture(t)
				files := []string{"f.txt"}
				if nested {
					files = append(files, "sub/g.txt")
				}
				for _, file := range files {
					f.write(tc.from + "/" + file)
				}
				f.start(nil)
				// Windows는 하위 폴더에 감시 핸들이 열린 폴더의 이름 바꾸기와 옮기기를 거부한다. fsnotify는 Windows에서
				// 하위 폴더까지 한 번에 감시하는 방법을 열어 두지 않아, 하위 폴더마다 핸들을 연다.
				if nested && runtime.GOOS == "windows" && !strings.HasPrefix(tc.from, "../") {
					err := os.Rename(f.path(tc.from), f.path(tc.to))
					if !errors.Is(err, fs.ErrPermission) {
						t.Fatalf("rename of a folder that holds a watched folder = %v, want access denied", err)
					}
					return
				}
				f.move(tc.from, tc.to)
				if got := f.changes(); !slices.Equal(got, tc.want) {
					t.Fatalf("hook events = %v, want %v", got, tc.want)
				}

				for _, file := range files {
					f.write(tc.to + "/" + file)
				}
				got := f.changes()
				movedIn := !strings.HasPrefix(tc.to, "../")
				for _, file := range files {
					want := "U " + tc.to + "/" + file
					if movedIn && !slices.Contains(got, want) {
						t.Errorf("change in the moved folder did not reach the hook as %q (got %v)", want, got)
					}
				}
				for _, ev := range got {
					if !movedIn || strings.Contains(ev, " "+tc.from+"/") {
						t.Errorf("change in the moved folder reached the hook as %q", ev)
					}
				}

				if !movedIn {
					f.assertNoWatchUnder(tc.from)
				}
			})
		}
	}
}

// 감시 대상 밖으로 옮긴 폴더와 같은 경로에 폴더를 다시 만들면, 그 안에 새로 생긴 파일의 이벤트를 받는지 확인한다.
// fsnotify의 kqueue 구현에 옛 폴더 안 항목의 감시가 남으면 같은 이름의 새 파일은 생성 이벤트가 오지 않는다.
func TestFolderRecreatedAfterMoveOutIsWatched(t *testing.T) {
	f := newRenameFixture(t)
	f.write("src/d/f.txt")
	f.start(nil)
	f.move("src/d", "../outside/d")
	if got, want := f.changes(), []string{"D src/d/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
	if err := os.Mkdir(f.path("src/d"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, want := f.changes(), []string{"C src/d/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
	f.write("src/d/f.txt")
	if got := f.changes(); !slices.Contains(got, "C src/d/f.txt") {
		t.Fatalf("creation in the recreated folder did not reach the hook (got %v)", got)
	}
}

// 감시 중인 폴더를 감시 대상 밖으로 옮기고 곧바로 지워도, runner가 멈추지 않고 옛 경로의 삭제(D)를 hook에
// 넘기는지 확인한다. fsnotify의 inotify 구현은 옮긴 폴더의 감시를 풀다 실패하면 오류를 보내는데, 그 폴더가 이미
// 지워졌으면 EINVAL로 실패한다. 이 오류를 보내는 동안 감시 루프가 Remove를 부르면 서로를 기다려 멈춘다.
func TestFolderRemovedRightAfterMoveOutKeepsWatching(t *testing.T) {
	f := newRenameFixture(t)
	f.start(nil)
	for range 10 {
		f.write("src/d/f.txt")
		f.changes()
		f.move("src/d", "../outside/d")
		if err := os.RemoveAll(filepath.Join(f.outside, "d")); err != nil {
			t.Fatal(err)
		}
		if got, want := f.changes(), []string{"D src/d/"}; !slices.Equal(got, want) {
			t.Fatalf("hook events = %v, want %v", got, want)
		}
	}
}

// 옛 경로가 exclude에 맞으면 이름을 바꾸거나 옮겨도 hook이 삭제(D)를 받지 않는지 확인한다.
func TestRenameFromExcludedPathDoesNotNotifyDelete(t *testing.T) {
	f := newRenameFixture(t)
	f.write("src/a.tmp")
	f.write("src/b.tmp")
	f.start(domain.ExcludePatterns{"**/*.tmp"})
	f.move("src/a.tmp", "src/a.txt")
	f.move("src/b.tmp", "../outside/b.tmp")
	if got, want := f.changes(), []string{"C src/a.txt"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
}

// 감시 대상 밖으로 옮긴 폴더의 옛 경로에 곧바로 폴더를 다시 만들어도, 옮겨 간 폴더 안의 변경은 hook을 실행하지
// 않고 새 폴더 안의 변경만 hook을 실행하는지 확인한다. Windows에서 옮겨 간 폴더의 감시가 남으면 두 폴더의
// 변경이 모두 옛 경로로 온다.
func TestFolderRecreatedRightAfterMoveOutIgnoresMovedFolder(t *testing.T) {
	f := newRenameFixture(t)
	f.write("src/d/moved.txt")
	f.start(nil)
	f.move("src/d", "../outside/d")
	if err := os.Mkdir(f.path("src/d"), 0700); err != nil {
		t.Fatal(err)
	}
	// 같은 경로에 폴더가 이미 다시 생겼어도, 옛 폴더를 감시 목록에 적어 두었으므로 옛 폴더의 D가 온다.
	if got, want := f.changes(), []string{"D src/d/", "C src/d/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
	f.write("../outside/d/moved.txt")
	f.write("../outside/d/other.txt")
	f.write("src/d/new.txt")
	got := f.changes()
	if !slices.Contains(got, "C src/d/new.txt") {
		t.Errorf("creation in the recreated folder did not reach the hook (got %v)", got)
	}
	for _, ev := range got {
		if strings.HasSuffix(ev, "/moved.txt") || strings.HasSuffix(ev, "/other.txt") {
			t.Errorf("change in the moved folder reached the hook as %q", ev)
		}
	}
	// 옮겨 간 폴더의 감시가 남으면 감시 목록에 옛 경로가 새 폴더와 함께 두 번 나온다.
	time.Sleep(3 * leakCheckDelay)
	f.changes()
	count := 0
	for _, p := range f.runner.tree.watcher.WatchList() {
		if filepath.Clean(p) == f.path("src/d") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("watch list holds src/d %d times, want once (%v)", count, f.runner.tree.watcher.WatchList())
	}
}

// 이름 바꾸기가 다른 파일 쓰기와 함께 빠르게 이어져도, 이름을 바꾼 파일마다 hook이 옛 경로의 삭제(D)를 새 경로의
// 생성(C)보다 먼저 받는지 확인한다. kqueue(macOS)는 새 경로의 Create를 옛 경로의 Rename보다 수십 ms 먼저
// 알리기도 한다.
func TestManyRenamesNotifyDeleteBeforeCreate(t *testing.T) {
	const renamers, renamesEach = 4, 50
	f := newRenameFixture(t)
	for k := 0; k < renamers; k++ {
		for i := 0; i < renamesEach; i++ {
			f.write(fmt.Sprintf("src/a-%d-%d.txt", k, i))
		}
	}
	f.start(nil)
	// 받은 이벤트가 hook 쪽 채널을 채워 감시 루프의 대기열이 넘치지 않도록 계속 받아 둔다.
	var mu sync.Mutex
	var got []string
	go func() {
		for ev := range f.changed {
			rel, err := filepath.Rel(f.target, ev.Path.String())
			if err != nil {
				continue
			}
			mu.Lock()
			got = append(got, ev.Operation.String()+" "+filepath.ToSlash(rel))
			mu.Unlock()
		}
	}()
	seen := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Contains(got, want)
	}

	stopWriting := make(chan struct{})
	writing := make(chan struct{})
	go func() {
		defer close(writing)
		for n := 0; ; n++ {
			select {
			case <-stopWriting:
				return
			default:
			}
			_ = os.WriteFile(f.path("dst/noise.txt"), []byte(fmt.Sprint(n)), 0600)
			time.Sleep(time.Millisecond)
		}
	}()
	var renaming sync.WaitGroup
	for k := 0; k < renamers; k++ {
		renaming.Add(1)
		go func(k int) {
			defer renaming.Done()
			for i := 0; i < renamesEach; i++ {
				if err := os.Rename(f.path(fmt.Sprintf("src/a-%d-%d.txt", k, i)), f.path(fmt.Sprintf("src/b-%d-%d.txt", k, i))); err != nil {
					t.Error(err)
				}
			}
		}(k)
	}
	renaming.Wait()
	close(stopWriting)
	<-writing
	f.write("src/zz-sentinel.txt")
	deadline := time.After(10 * time.Second)
	for !seen("C src/zz-sentinel.txt") {
		select {
		case err := <-f.result:
			t.Fatalf("run stopped: %v", err)
		case <-deadline:
			t.Fatal("sentinel did not reach the hook")
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	position := map[string]int{}
	for i, ev := range got {
		position[ev] = i
	}
	for k := 0; k < renamers; k++ {
		for i := 0; i < renamesEach; i++ {
			d, okD := position[fmt.Sprintf("D src/a-%d-%d.txt", k, i)]
			c, okC := position[fmt.Sprintf("C src/b-%d-%d.txt", k, i)]
			if !okD || !okC || d > c {
				t.Errorf("rename %d-%d: D at %d (%v), C at %d (%v)", k, i, d, okD, c, okC)
			}
		}
	}
}

// 옛 경로의 Rename을 기다리다 보류 시간이 끝나 넘긴 Create가 watcher를 다시 만들어도, 예전 watcher가 닫히기 전에
// 받은 이벤트가 새 watcher가 받은 다음 이벤트보다 먼저 hook에 가는지 확인한다.
//
// 보류는 kqueue에서만, 감시를 풀지 못한 폴더는 주로 Windows에서 생기므로 두 상태를 직접 만든다. 감시 대상 밖
// 폴더의 감시를 남겨, 풀지 못한 폴더 d의 Create가 rebuildIfLeaked에서 watcher를 다시 만들게 한다.
func TestRetiredWatcherEventsPrecedeNextEventAfterHoldTimeout(t *testing.T) {
	target, outside := t.TempDir(), t.TempDir()
	dir := filepath.Join(target, "d")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	option := watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchEvent{Create: true}, DirEvent: domain.WatchEvent{Create: true}})
	// 폴더 이벤트를 모으지 않아, 보류를 푼 d의 C가 묶음을 기다리지 않고 곧바로 hook에 간다.
	option.DirBatch = domain.DirBatchOption{}
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(target), Option: option}, newMockLogger(), process.CommandMode{})
	if err := r.tree.watcher.Add(outside); err != nil {
		t.Fatal(err)
	}
	retired, retiredErrors := controlledSource(r)
	holdTimeout := make(chan time.Time)
	r.renamePairer.held = []fsnotify.Event{{Name: dir, Op: fsnotify.Create}}
	r.renamePairer.source = filepath.Join(target, "old")
	r.renamePairer.timeout = holdTimeout
	r.tree.unreleasedDirs[dir] = struct{}{}
	changed, result := startRunner(t, r, nil)

	var got []string
	await := func(want string) {
		t.Helper()
		for !slices.Contains(got, want) {
			ev := awaitHookEvent(t, changed, result)
			if rel, err := filepath.Rel(target, ev.Path.String()); err == nil {
				got = append(got, filepath.ToSlash(rel))
			}
		}
	}
	holdTimeout <- time.Now()
	await("d")
	// 감시 루프가 예전 watcher의 이벤트를 새 watcher의 다음 이벤트 뒤에 받으면, 그때까지 보내기가 멈추므로 따로 보낸다.
	go func() {
		retired <- fsnotify.Event{Name: filepath.Join(target, "retired.txt"), Op: fsnotify.Create}
		close(retired)
		close(retiredErrors)
	}()
	if err := os.WriteFile(filepath.Join(target, "next.txt"), []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	await("retired.txt")
	await("next.txt")
	if slices.Index(got, "retired.txt") > slices.Index(got, "next.txt") {
		t.Fatalf("event of the retired watcher reached the hook after the next event: %v", got)
	}
}

// 폴더가 감시 시작 전에 있었는지, 감시 중에 만들어졌는지, 감시 중에 통째로 옮겨 왔는지와
// 관계없이 같은 규칙으로 감시되는지 확인한다.
func TestSubDirWatchRuleIsSameAtStartAndWhileWatching(t *testing.T) {
	cases := []struct {
		name    string
		dirs    []string // 감시 대상 안에 만들 폴더
		absLink string   // 감시 대상 밖 폴더를 절대 경로로 가리키는 링크의 이름
		relLink string   // 감시 대상 밖 폴더를 상대 경로로 가리키는 링크의 이름
		exclude []string
		probe   string // 내용을 쓸 파일
		want    bool   // probe의 변경으로 hook이 실행되어야 하는지
	}{
		{name: "plain nested folder", dirs: []string{"plain/sub"}, probe: "plain/sub/a.txt", want: true},
		{name: "default-excluded folder", dirs: []string{".cache/sub"}, probe: ".cache/sub/a.txt", want: false},
		{name: "default-excluded nested folder", dirs: []string{"plain/_tmp"}, probe: "plain/_tmp/a.txt", want: false},
		{name: "exclude sibling with same prefix", dirs: []string{"dir1", "dir10"}, exclude: []string{"dir1/**"}, probe: "dir10/a.txt", want: true},
		{name: "exclude subfolder", dirs: []string{"parent/sub/deep"}, exclude: []string{"parent/sub/**"}, probe: "parent/sub/deep/a.txt", want: false},
		{name: "exclude folder at any depth", dirs: []string{"a/node_modules/pkg"}, exclude: []string{"**/node_modules/**"}, probe: "a/node_modules/pkg/a.txt", want: false},
		{name: "absolute symlink to folder", absLink: "link", probe: "link/a.txt", want: false},
		{name: "relative symlink to folder", relLink: "link", probe: "link/a.txt", want: false},
		{name: "absolute and relative symlinks to folder", absLink: "abs", relLink: "rel", probe: "abs/a.txt", want: false},
	}
	timings := []struct {
		name string
		// 폴더를 만드는 시점과 방법이다. start는 감시 시작 전에, 나머지는 감시 중에 만든다.
		start bool
		moved bool
	}{
		{name: "existing at start", start: true},
		{name: "created while watching"},
		{name: "moved in while watching", moved: true},
	}
	for _, tc := range cases {
		for _, timing := range timings {
			t.Run(tc.name+"/"+timing.name, func(t *testing.T) {
				base := t.TempDir()
				target, staging, outside := filepath.Join(base, "target"), filepath.Join(base, "staging"), filepath.Join(base, "outside")
				for _, dir := range []string{target, staging, outside} {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				build := func(root string) {
					for _, dir := range tc.dirs {
						if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
							t.Fatal(err)
						}
					}
					for _, link := range []struct{ name, dest string }{
						{tc.absLink, outside},
						{tc.relLink, filepath.Join("..", "outside")},
					} {
						if link.name == "" {
							continue
						}
						if err := os.Symlink(link.dest, filepath.Join(root, link.name)); err != nil {
							t.Skipf("symlink not available: %v", err)
						}
					}
				}
				if tc.relLink != "" {
					// fsnotify의 kqueue 구현(macOS)은 상대 경로 링크를 작업 디렉터리 기준으로 풀어, 그 위치에
					// 폴더가 있을 때만 따라간다. 링크가 가리키는 폴더와 같은 폴더로 풀리도록 작업 디렉터리를 옮긴다.
					chdir(t, target)
				}
				if timing.start {
					build(target)
				}

				r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(target), Option: watchTargetOption(domain.WatchOption{SubDir: true, Exclude: tc.exclude, FileEvent: domain.WatchEvent{Create: true, Update: true}})}, newMockLogger(), process.CommandMode{})
				changed, result := startRunner(t, r, nil)

				seen := map[string]bool{}
				write := func(rel string) {
					if err := os.WriteFile(filepath.Join(target, rel), []byte("event"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				await := func(rel string) {
					for !seen[rel] {
						got := awaitHookEvent(t, changed, result)
						gotAbs, err := filepath.Abs(got.Path.String())
						if err != nil {
							t.Fatal(err)
						}
						gotRel, err := filepath.Rel(target, gotAbs)
						if got.Path == "" || err != nil || gotRel == ".." || strings.HasPrefix(gotRel, ".."+string(filepath.Separator)) {
							t.Fatalf("event outside the watch target ran the hook: %q", got.Path)
						}
						seen[filepath.ToSlash(gotRel)] = true
					}
				}

				if !timing.start {
					if timing.moved {
						build(staging)
						entries, err := os.ReadDir(staging)
						if err != nil {
							t.Fatal(err)
						}
						for _, entry := range entries {
							if err := os.Rename(filepath.Join(staging, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
								t.Fatal(err)
							}
						}
					} else {
						build(target)
					}
				}
				// 감시 루프는 이벤트를 받은 순서대로 처리하므로, 뒤이어 만든 파일의 hook이 실행됐다면
				// 앞서 생긴 폴더의 감시 목록 반영도 끝났다. kqueue는 새 항목을 이름 순으로 알리므로
				// 다른 항목보다 뒤에 정렬되는 이름을 쓴다.
				write("zz-registered.txt")
				await("zz-registered.txt")

				write(tc.probe)
				if tc.want {
					await(tc.probe)
					return
				}
				// 제외된 변경이 오지 않았음을 시간 초과만으로 판단하지 않도록, 뒤이어 만든 파일의 hook이
				// 실행될 때까지 받은 경로를 확인한다.
				write("zz-after-probe.txt")
				await("zz-after-probe.txt")
				if seen[tc.probe] {
					t.Fatalf("change of %s ran the hook", tc.probe)
				}
			})
		}
	}
}

// 감시 대상 path가 폴더를 가리키는 링크이면 subDir와 관계없이 링크가 가리키는 폴더를 감시하고,
// exclude 패턴도 그 폴더의 같은 위치를 빼는지 확인한다.
func TestLinkedTargetPathWatchesLinkedFolder(t *testing.T) {
	cases := []struct {
		name      string
		subDir    bool
		startDirs []string // 감시 시작 전에 링크가 가리키는 폴더 안에 만들 폴더
		laterDirs []string // 감시 중에 링크가 가리키는 폴더 안에 만들 폴더
		exclude   []string
		probe     string
		want      bool
	}{
		{name: "file in linked folder without subDir", probe: "a.txt", want: true},
		{name: "file in linked folder with subDir", subDir: true, probe: "a.txt", want: true},
		{name: "existing subfolder", subDir: true, startDirs: []string{"sub"}, probe: "sub/a.txt", want: true},
		{name: "subfolder created while watching", subDir: true, laterDirs: []string{"sub"}, probe: "sub/a.txt", want: true},
		// exclude 패턴이 있으면 감시 루프가 새 폴더를 감시 목록에 넣기까지 시간이 늘어, macOS에서 reAddDirs가
		// 되돌리는 fsnotify의 등록 경합이 자주 일어난다.
		{name: "subfolder created while watching with exclude", subDir: true, laterDirs: []string{"sub"}, exclude: []string{"other/**"}, probe: "sub/a.txt", want: true},
		{name: "existing excluded folder", subDir: true, startDirs: []string{"ex"}, exclude: []string{"ex/**"}, probe: "ex/a.txt", want: false},
		{name: "excluded folder created while watching", subDir: true, laterDirs: []string{"ex"}, exclude: []string{"ex/**"}, probe: "ex/a.txt", want: false},
		{name: "excluded file", subDir: true, exclude: []string{"skip.txt"}, probe: "skip.txt", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			linked, link := filepath.Join(base, "linked"), filepath.Join(base, "link")
			if err := os.Mkdir(linked, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(linked, link); err != nil {
				t.Skipf("symlink not available: %v", err)
			}
			// 이벤트 경로는 링크를 모두 푼 폴더 기준으로 온다. macOS의 TempDir처럼 상위 경로에 링크가 있어도 맞춘다.
			resolved, err := filepath.EvalSymlinks(linked)
			if err != nil {
				t.Fatal(err)
			}
			makeDirs := func(dirs []string) {
				for _, dir := range dirs {
					if err := os.MkdirAll(filepath.Join(linked, dir), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			makeDirs(tc.startDirs)

			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(link), Option: watchTargetOption(domain.WatchOption{SubDir: tc.subDir, Exclude: tc.exclude, FileEvent: domain.WatchEvent{Create: true, Update: true}})}, newMockLogger(), process.CommandMode{})
			changed, result := startRunner(t, r, nil)

			seen := map[string]bool{}
			write := func(rel string) {
				if err := os.WriteFile(filepath.Join(linked, rel), []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			await := func(rel string) {
				for !seen[rel] {
					got := awaitHookEvent(t, changed, result)
					gotRel, err := filepath.Rel(resolved, got.Path.String())
					if err != nil {
						t.Fatal(err)
					}
					seen[filepath.ToSlash(gotRel)] = true
				}
			}

			makeDirs(tc.laterDirs)
			// 감시 루프는 이벤트를 받은 순서대로 처리하므로, 뒤이어 만든 파일의 hook이 실행됐다면
			// 앞서 생긴 폴더의 감시 목록 반영도 끝났다.
			write("zz-registered.txt")
			await("zz-registered.txt")

			write(tc.probe)
			if tc.want {
				await(tc.probe)
				return
			}
			write("zz-after-probe.txt")
			await("zz-after-probe.txt")
			if seen[tc.probe] {
				t.Fatalf("change of %s ran the hook", tc.probe)
			}
		})
	}
}

func TestNativeWatcherStopReleasesDirectory(t *testing.T) {
	for _, recursive := range []bool{false, true} {
		t.Run(fmt.Sprintf("subDir=%t", recursive), func(t *testing.T) {
			path := t.TempDir()
			if err := os.Mkdir(filepath.Join(path, "child"), 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: watchTargetOption(domain.WatchOption{SubDir: recursive, FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger(), process.CommandMode{})
			assertWatchDirectoriesRemovable(t, r)
			changed, result := startRunner(t, r, nil)
			// finish hook은 Stop을 부른 고루틴이 실행하므로, 감시를 시작한 뒤에 바꿔도 Run과 겹치지 않는다.
			finished := make(chan struct{})
			r.dispatcher.hooks.runFinishHook = func() { close(finished) }
			if err := os.WriteFile(filepath.Join(path, "actual.txt"), []byte("event"), 0600); err != nil {
				t.Fatal(err)
			}
			awaitHookEvent(t, changed, nil)
			var wg sync.WaitGroup
			wg.Add(1)
			r.Stop(&wg)
			if err := <-result; err != nil {
				t.Fatalf("normal stop: %v", err)
			}
			select {
			case <-finished:
			default:
				t.Fatal("Stop returned before finish hook")
			}
		})
	}
}

func TestSubDirControlsExistingSubdirectoryEvents(t *testing.T) {
	for _, subDir := range []bool{false, true} {
		t.Run(fmt.Sprintf("subDir=%t", subDir), func(t *testing.T) {
			path := t.TempDir()
			child := filepath.Join(path, "child")
			if err := os.Mkdir(child, 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: watchTargetOption(domain.WatchOption{SubDir: subDir, FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger(), process.CommandMode{})
			assertWatchDirectoriesRemovable(t, r)
			changed, _ := startRunner(t, r, nil)

			nested := filepath.Join(child, "nested.txt")
			if err := os.WriteFile(nested, []byte("event"), 0600); err != nil {
				t.Fatal(err)
			}
			// 하위 디렉터리 이벤트가 오지 않았음을 시간 초과만으로 판단하지 않도록,
			// 이어서 루트에 파일을 만들고 그 이벤트가 도착할 때까지 받은 경로를 확인한다.
			sentinel := filepath.Join(path, "sentinel.txt")
			if err := os.WriteFile(sentinel, []byte("event"), 0600); err != nil {
				t.Fatal(err)
			}
			nestedSeen, sentinelSeen := false, false
			for !sentinelSeen || (subDir && !nestedSeen) {
				got := awaitHookEvent(t, changed, nil)
				nestedSeen = nestedSeen || got.Path.String() == nested
				sentinelSeen = sentinelSeen || got.Path.String() == sentinel
			}
			if !subDir && nestedSeen {
				t.Fatal("change in existing subdirectory ran the hook with subDir=false")
			}
		})
	}
}

func TestStopDiscardsPendingEvents(t *testing.T) {
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger(), process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r.dispatcher.hooks.runStartHook = func() {}
	r.dispatcher.hooks.runFinishHook = func() {}
	r.dispatcher.hooks.runChangeHook = func(domain.Event) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	go r.Run()
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "first"), Op: fsnotify.Create}
	<-entered
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "queued"), Op: fsnotify.Create}
	var wg sync.WaitGroup
	wg.Add(1)
	stopped := make(chan struct{})
	go func() { r.Stop(&wg); close(stopped) }()
	time.Sleep(40 * time.Millisecond)
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop blocked behind pending event")
	}
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("shutdown started pending hook: calls=%d", got)
	}
}

func TestWatcherLossStopsDispatchWhileHookRuns(t *testing.T) {
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger(), process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r)
	events, watchErrors := controlledSource(r)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	r.dispatcher.hooks.runStartHook = func() {}
	r.dispatcher.hooks.runFinishHook = func() {}
	r.dispatcher.hooks.runChangeHook = func(domain.Event) { close(entered); <-release }
	go func() { r.Run(); close(done) }()
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "first"), Op: fsnotify.Create}
	<-entered
	watchErrors <- fsnotify.ErrEventOverflow
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("watcher loss was ignored while hook ran")
	}
	close(release)
	var wg sync.WaitGroup
	wg.Add(1)
	r.Stop(&wg)
}

// kqueue에서 fsnotify가 오류 채널로 ErrNotExist를 보내도 감시를 계속하고, 다른 운영체제에서는 그 오류로 감시를
// 멈추는지 확인한다.
func TestWatcherNotExistErrorKeepsWatchingOnlyOnKqueue(t *testing.T) {
	root := t.TempDir()
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger(), process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r)
	events, watchErrors := controlledSource(r)
	runs, result := startRunner(t, r, nil)
	// fsnotify의 kqueue 구현은 지운 파일의 경로에 다시 생긴 파일을 감시하려다 그 파일마저 사라지면 ENOENT를 그대로 보낸다.
	watchErrors <- syscall.ENOENT
	if !kqueueOS {
		select {
		case err := <-result:
			var fatal *FatalError
			if !errors.As(err, &fatal) || !errors.Is(err, syscall.ENOENT) {
				t.Fatalf("run stopped with %v, want a FatalError wrapping ENOENT", err)
			}
		case <-time.After(time.Second):
			t.Fatal("ENOENT from the watcher did not stop the run")
		}
		return
	}
	after := filepath.Join(root, "after.txt")
	writeFile(t, after)
	select {
	case events <- fsnotify.Event{Name: after, Op: fsnotify.Create}:
	case err := <-result:
		t.Fatalf("run stopped: %v", err)
	}
	awaitHookPath(t, runs, result, after)
}

func TestBoundedDispatchReportsLoss(t *testing.T) {
	const boundedQueueSize = 16
	option := watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})
	option.Queue.Size = boundedQueueSize
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: option}, newMockLogger(), process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	r.dispatcher.hooks.runStartHook = func() {}
	r.dispatcher.hooks.runFinishHook = func() {}
	// 실제 runChangeHook은 hook을 시작하면 돌아오고, 자리가 없으면 자리가 날 때까지 막힌다. 그래서 첫 실행은 돌고 있는
	// 실행처럼 돌아오고, 다음 실행은 자리를 기다리며 대기열에 남는다.
	var calls atomic.Int32
	r.dispatcher.hooks.runChangeHook = func(domain.Event) {
		if calls.Add(1) == 1 {
			close(entered)
			return
		}
		<-release
	}
	baseline := runtime.NumGoroutine()
	go func() { result <- r.Run() }()
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "first"), Op: fsnotify.Create}
	<-entered
	started := time.Now()
	peak := runtime.NumGoroutine()
	for n := range boundedQueueSize + 1 {
		events <- fsnotify.Event{Name: filepath.Join(r.path.String(), fmt.Sprint(n)), Op: fsnotify.Create}
		if current := runtime.NumGoroutine(); current > peak {
			peak = current
		}
	}
	select {
	case err := <-result:
		var fatal *FatalError
		if !errors.As(err, &fatal) || !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full (capacity 16)") {
			t.Fatalf("bad loss: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queue overflow blocked")
	}
	if growth := peak - baseline; growth > 4 {
		t.Errorf("unbounded goroutines: %d", growth)
	}
	t.Logf("pending capacity=%d active=1 goroutine baseline=%d sampled-peak=%d loss-latency=%s", boundedQueueSize, baseline, peak, time.Since(started))
	close(release)
	var wg sync.WaitGroup
	wg.Add(1)
	r.Stop(&wg)
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if got := runtime.NumGoroutine(); got > baseline {
		t.Errorf("goroutines left: baseline=%d after=%d", baseline, got)
	}
	t.Logf("goroutines after stop=%d baseline=%d", runtime.NumGoroutine(), baseline)
}

// 기다린 뒤 last로 넘기면 마지막 유효 이벤트 하나만, 기다리지 않고 each로 넘기면 모든 이벤트가 생긴 순서대로 hook에
// 가는지, 그리고 run.concurrency가 1이면 run.interrupt가 none이든 oldest든 hook이 겹쳐 실행되지 않는지 확인한다.
func TestControlledModesPreservePolicy(t *testing.T) {
	modes := []struct {
		name    string
		wait    domain.Millisecond
		deliver domain.Deliver
	}{
		{"waitMillisecond=100/deliver=last", 100, domain.DeliverLast},
		{"waitMillisecond=0/deliver=each", 0, domain.DeliverEach},
	}
	for _, mode := range modes {
		for name, interrupt := range map[string]domain.Interrupt{"none": domain.InterruptNone, "oldest": domain.InterruptOldest} {
			t.Run(fmt.Sprintf("%s/interrupt=%s", mode.name, name), func(t *testing.T) {
				oldest := interrupt == domain.InterruptOldest
				helper.GlobalLogger = newMockLogger()
				each := mode.deliver == domain.DeliverEach
				var mu sync.Mutex
				var started, completed []string
				active, peak := 0, 0
				changed := make(chan struct{}, 64)
				began := make(chan struct{}, 64)
				command := runnerCommand(func(ctx *process.WatcherContext, event *domain.Event) error {
					mu.Lock()
					active++
					if active > peak {
						peak = active
					}
					started = append(started, filepath.Base(event.Path.String()))
					mu.Unlock()
					began <- struct{}{}
					defer func() { mu.Lock(); active--; mu.Unlock() }()
					select {
					case <-ctx.Context().Done():
						return ctx.Context().Err()
					case <-time.After(5 * time.Millisecond):
						mu.Lock()
						completed = append(completed, filepath.Base(event.Path.String()))
						mu.Unlock()
						changed <- struct{}{}
						return nil
					}
				})
				empty := emptyCommand()
				option := hookBatchOption(mode.wait, 1000, mode.deliver)
				option.Watch.Exclude = domain.ExcludePatterns{"**/*.swp"}
				option.Run = domain.RunPolicy{Concurrency: 1, Interrupt: interrupt}
				root := t.TempDir()
				r := NewWatchTargetRunner(
					domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnBeforeChange: empty, OnAfterChange: empty}},
					domain.WatchTargetsCommandSet{Path: domain.Path(root), LifeCycle: domain.WatchTargetsLifeCycle{OnStartWatch: empty, OnChange: command, OnFinishWatch: empty}, Option: option},
					newMockLogger(), process.CommandMode{})
				events, _ := controlledSource(r)
				go r.Run()
				// 파일을 모두 만든 뒤 이벤트를 몰아 넣어, last의 이벤트가 한 번 기다리는 동안 모두 모이게 한다.
				var names []string
				for n := range 24 {
					names = append(names, filepath.Join(root, fmt.Sprintf("%02d.go", n)))
					writeFile(t, names[n])
				}
				for _, name := range names {
					events <- fsnotify.Event{Name: name, Op: fsnotify.Create}
					if each && oldest {
						<-began
					}
					events <- fsnotify.Event{Name: filepath.Join(root, "excluded.swp"), Op: fsnotify.Create}
				}
				deadline := time.After(3 * time.Second)
				for {
					mu.Lock()
					last := len(completed) > 0 && completed[len(completed)-1] == "23.go"
					mu.Unlock()
					if last {
						break
					}
					select {
					case <-changed:
					case <-deadline:
						t.Fatal("last valid event did not complete")
					}
				}
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
				mu.Lock()
				defer mu.Unlock()
				if peak != 1 || active != 0 {
					t.Fatalf("overlapping/leaked execution: peak=%d active=%d", peak, active)
				}
				if !each {
					if len(started) != 1 || started[0] != "23.go" {
						t.Fatalf("last: %v", started)
					}
				} else {
					if len(started) != 24 {
						t.Fatalf("each dropped work: %v", started)
					}
					for n, path := range started {
						if path != fmt.Sprintf("%02d.go", n) {
							t.Fatalf("reordered: %v", started)
						}
					}
					if !oldest && len(completed) != 24 {
						t.Fatalf("sequential work canceled: %v", completed)
					}
				}
				t.Logf("started=%d completed=%d peak=%d excluded=24", len(started), len(completed), peak)
			})
		}
	}
}

// 경고 hook이 끝나지 않은 동안에도 감시 루프는 이벤트를 받아 hook을 실행하고, 그동안 생긴 경고는 가장 최근 것
// 하나만 경고 hook이 끝난 뒤 실행한다.
func TestSlowQueueWarningKeepsOnlyLatestAndDoesNotBlockWatching(t *testing.T) {
	root := t.TempDir()
	warned, releaseWarning := make(chan struct{}, 8), make(chan struct{})
	var mu sync.Mutex
	var warnings []string
	global := domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnQueueWarning: runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
		env := hookEnvironment(ctx)
		mu.Lock()
		warnings = append(warnings, env["DW_QUEUE_LEVEL"]+" "+env["DW_QUEUE_DIRECTION"])
		mu.Unlock()
		warned <- struct{}{}
		<-releaseWarning
		return nil
	})}}
	option := hookBatchOption(0, 1000, domain.DeliverEach)
	option.Queue.Size = 10
	r := NewWatchTargetRunner(global, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: option}, newMockLogger(), process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, releaseChange, changed := make(chan struct{}), make(chan struct{}), make(chan string, 16)
	r.dispatcher.hooks.runStartHook = func() {}
	r.dispatcher.hooks.runFinishHook = func() {}
	r.dispatcher.hooks.runChangeHook = func(e domain.Event) {
		if filepath.Base(e.Path.String()) == "first" {
			close(entered)
			<-releaseChange
		}
		changed <- filepath.Base(e.Path.String())
	}
	result := make(chan error, 1)
	go func() { result <- r.Run() }()
	send := func(name string) {
		path := filepath.Join(root, name)
		writeFile(t, path)
		select {
		case events <- fsnotify.Event{Name: path, Op: fsnotify.Create}:
		case err := <-result:
			t.Fatalf("run stopped: %v", err)
		case <-time.After(time.Second):
			t.Fatal("watch loop stopped receiving events")
		}
	}
	send("first")
	<-entered
	// first가 자리를 기다리며 대기열에 남아 있으므로, 9개를 더 넣으면 기다리는 수가 10이 되어 70과 90에 닿는다.
	// 70 경고 hook이 끝나지 않았으므로 90 경고는 남겨 둔다.
	for n := range 9 {
		send(fmt.Sprint(n))
	}
	select {
	case <-warned:
	case <-time.After(time.Second):
		t.Fatal("queue warning hook did not run")
	}
	// 대기열이 비면 90과 70의 내려감이 생기고, 남겨 둔 경고는 가장 최근인 70 내려감으로 바뀐다.
	close(releaseChange)
	for range 10 {
		select {
		case <-changed:
		case <-time.After(time.Second):
			t.Fatal("watch loop stopped while queue warning hook ran")
		}
	}
	send("after")
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("watch loop stopped while queue warning hook ran")
	}
	close(releaseWarning)
	select {
	case <-warned:
	case <-time.After(time.Second):
		t.Fatal("pending queue warning did not run")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	r.Stop(&wg)
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"70 up", "70 down"}; !slices.Equal(warnings, want) {
		t.Fatalf("queue warnings = %q, want %q", warnings, want)
	}
}
