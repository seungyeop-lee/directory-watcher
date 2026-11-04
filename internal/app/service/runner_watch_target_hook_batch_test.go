package service

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// hookBatchFixture는 감시 대상 root와, root의 변경을 받는 hookBatcher를 둔다. 감시 루프 없이 handleEvent가 넘기는
// 변경을 add로 직접 넣는다.
type hookBatchFixture struct {
	t    *testing.T
	root string
	b    *hookBatcher
}

func newHookBatchFixture(t *testing.T, root string, option domain.WatchTargetsOption) *hookBatchFixture {
	return &hookBatchFixture{t: t, root: root, b: newHookBatcher(option)}
}

func (f *hookBatchFixture) path(rel string) string {
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

// add는 감시 대상 기준 상대 경로 rel의 변경 op를 hookBatcher에 넣는다. rel이 /로 끝나면 폴더의 변경이다. existed는
// handleEvent가 이 변경을 기록에 반영하기 직전에 경로가 기록에 있었는지다. 기다리는 동안에는 실행이 나오지 않아야 한다.
func (f *hookBatchFixture) add(op fsnotify.Op, rel string, existed bool) {
	f.t.Helper()
	event := domain.Event{Path: domain.Path(f.path(strings.TrimSuffix(rel, "/"))), Operation: domain.Operation(op), IsDir: strings.HasSuffix(rel, "/")}
	if runs := f.b.add(change{Event: event, existed: existed}); runs != nil {
		f.t.Fatalf("add(%v %s) returned runs %v while waiting", op, rel, runs)
	}
}

// describe는 실행 runs를 describeRun의 꼴로 돌려준다.
func (f *hookBatchFixture) describe(runs []domain.Event) []string {
	f.t.Helper()
	got := []string{}
	for _, run := range runs {
		got = append(got, describeRun(f.t, f.root, run))
	}
	return got
}

// flushWhenDue는 기다리는 시간이 끝났다고 Due가 알리면 flush가 돌려준 실행을 describe의 꼴로 돌려준다.
func (f *hookBatchFixture) flushWhenDue() []string {
	f.t.Helper()
	select {
	case <-f.b.Due():
	case <-time.After(5 * time.Second):
		f.t.Fatal("hook batch did not become due")
	}
	return f.describe(f.b.flush())
}

// 한 번 기다리는 동안 모인 변경을 deliver 값마다 정해진 실행으로 합치는지 확인한다. x.swp는 생겼다가 사라져
// 최종 변화가 없다.
func TestDeliverTurnsOneWaitIntoRuns(t *testing.T) {
	cases := []struct {
		deliver domain.Deliver
		name    string
		want    []string
	}{
		{domain.DeliverLast, "last", []string{"U b.txt"}},
		{domain.DeliverLastPerPath, "lastPerPath", []string{"C a.txt", "U b.txt"}},
		{domain.DeliverEach, "each", []string{"C a.txt", "U a.txt", "U b.txt", "C x.swp", "D x.swp"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "b.txt"))
			f := newHookBatchFixture(t, root, hookBatchOption(50, 1000, tc.deliver))
			writeFile(t, f.path("a.txt"))
			f.add(fsnotify.Create, "a.txt", false)
			f.add(fsnotify.Write, "a.txt", true)
			f.add(fsnotify.Write, "b.txt", true)
			f.add(fsnotify.Create, "x.swp", false)
			f.add(fsnotify.Remove, "x.swp", true)
			if got := f.flushWhenDue(); !slices.Equal(got, tc.want) {
				t.Fatalf("runs = %v, want %v", got, tc.want)
			}
		})
	}
}

// last와 lastPerPath가 경로의 최종 변화를 이전 기록과 지금 디스크 상태로 정하는지 확인한다.
func TestFinalChangeComparesRecordWithDisk(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
		act   func(f *hookBatchFixture)
		want  []string
	}{
		{
			name: "file created",
			act:  func(f *hookBatchFixture) { writeFile(f.t, f.path("f.txt")); f.add(fsnotify.Create, "f.txt", false) },
			want: []string{"C f.txt"},
		},
		{
			name: "file created without its create event",
			act:  func(f *hookBatchFixture) { writeFile(f.t, f.path("f.txt")); f.add(fsnotify.Write, "f.txt", false) },
			want: []string{"C f.txt"},
		},
		{
			name:  "file modified",
			setup: func(t *testing.T, root string) { writeFile(t, filepath.Join(root, "f.txt")) },
			act:   func(f *hookBatchFixture) { writeFile(f.t, f.path("f.txt")); f.add(fsnotify.Write, "f.txt", true) },
			want:  []string{"U f.txt"},
		},
		{
			name:  "file deleted",
			setup: func(t *testing.T, root string) { writeFile(t, filepath.Join(root, "f.txt")) },
			act: func(f *hookBatchFixture) {
				removeAll(f.t, f.path("f.txt"))
				f.add(fsnotify.Remove, "f.txt", true)
			},
			want: []string{"D f.txt"},
		},
		{
			name: "file created and deleted",
			act: func(f *hookBatchFixture) {
				writeFile(f.t, f.path("f.txt"))
				f.add(fsnotify.Create, "f.txt", false)
				removeAll(f.t, f.path("f.txt"))
				f.add(fsnotify.Remove, "f.txt", true)
			},
			want: []string{},
		},
		{
			name: "folder created",
			act:  func(f *hookBatchFixture) { mkdir(f.t, f.path("d")); f.add(fsnotify.Create, "d/", false) },
			want: []string{"C d/"},
		},
		{
			name:  "folder deleted and created again",
			setup: func(t *testing.T, root string) { mkdir(t, filepath.Join(root, "d")) },
			act: func(f *hookBatchFixture) {
				removeAll(f.t, f.path("d"))
				f.add(fsnotify.Remove, "d/", true)
				mkdir(f.t, f.path("d"))
				f.add(fsnotify.Create, "d/", false)
			},
			want: []string{},
		},
		{
			name:  "folder deleted",
			setup: func(t *testing.T, root string) { mkdir(t, filepath.Join(root, "d")) },
			act: func(f *hookBatchFixture) {
				removeAll(f.t, f.path("d"))
				f.add(fsnotify.Remove, "d/", true)
			},
			want: []string{"D d/"},
		},
		{
			name: "folder created and deleted",
			act: func(f *hookBatchFixture) {
				mkdir(f.t, f.path("d"))
				f.add(fsnotify.Create, "d/", false)
				removeAll(f.t, f.path("d"))
				f.add(fsnotify.Remove, "d/", true)
			},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, root)
			}
			f := newHookBatchFixture(t, root, hookBatchOption(50, 1000, domain.DeliverLastPerPath))
			tc.act(f)
			if got := f.flushWhenDue(); !slices.Equal(got, tc.want) {
				t.Fatalf("runs = %v, want %v", got, tc.want)
			}
		})
	}
}

// waitMillisecond보다 짧은 간격으로 변경이 계속 들어와도, 첫 변경부터 maxWaitMillisecond 안에 기다리기가 끝나는지 확인한다.
func TestMaxWaitEndsWaitDuringContinuousChanges(t *testing.T) {
	const maxWait = 500 * time.Millisecond
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"))
	f := newHookBatchFixture(t, root, hookBatchOption(100, domain.Millisecond(maxWait/time.Millisecond), domain.DeliverLast))
	start := time.Now()
	f.add(fsnotify.Write, "a.txt", true)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-f.b.Due():
			// 느린 환경에서 타이머가 늦게 알리는 만큼 여유를 둔다.
			if elapsed := time.Since(start); elapsed > maxWait+300*time.Millisecond {
				t.Errorf("wait ended after %v, want within %v", elapsed, maxWait)
			}
			if got, want := f.describe(f.b.flush()), []string{"U a.txt"}; !slices.Equal(got, want) {
				t.Fatalf("runs = %v, want %v", got, want)
			}
			return
		case <-time.After(50 * time.Millisecond):
			f.add(fsnotify.Write, "a.txt", true)
		case <-deadline:
			t.Fatal("wait did not end while changes kept coming")
		}
	}
}

// 폴더 삭제가 폴더 이벤트 묶음 두 개로 나뉘어 폴더의 D와 그 안의 D가 따로 들어와 한 번 기다리는 동안 모이면,
// lastPerPath는 최종 변화가 D인 폴더 아래의 D를 넘기지 않는지 확인한다. last와 each의 결과는 hookBatch가 폴더
// 이벤트를 흡수하지 않을 때와 같다.
func TestLastPerPathDropsDeletesUnderDeletedFolder(t *testing.T) {
	cases := []struct {
		deliver  domain.Deliver
		name     string
		dirEvent domain.WatchEvent
		want     []string
	}{
		{domain.DeliverLastPerPath, "lastPerPath", domain.WatchEvent{Create: true, Delete: true}, []string{"D d/"}},
		{domain.DeliverLastPerPath, "lastPerPath with dirEvent none", domain.WatchEvent{}, []string{}},
		{domain.DeliverLast, "last", domain.WatchEvent{Create: true, Delete: true}, []string{"D d/"}},
		{domain.DeliverEach, "each", domain.WatchEvent{Create: true, Delete: true}, []string{"D d/sub/g.txt", "D d/f.txt", "D d/sub/", "D d/"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mkdir(t, filepath.Join(root, "d"))
			mkdir(t, filepath.Join(root, "d", "sub"))
			writeFile(t, filepath.Join(root, "d", "f.txt"))
			writeFile(t, filepath.Join(root, "d", "sub", "g.txt"))
			option := hookBatchOption(300, 2000, tc.deliver)
			option.Watch.DirEvent = tc.dirEvent
			f := newHookBatchFixture(t, root, option)
			removeAll(t, f.path("d"))
			f.add(fsnotify.Remove, "d/sub/g.txt", true)
			f.add(fsnotify.Remove, "d/f.txt", true)
			f.add(fsnotify.Remove, "d/sub/", true)
			f.add(fsnotify.Remove, "d/", true)
			if got := f.flushWhenDue(); !slices.Equal(got, tc.want) {
				t.Fatalf("runs = %v, want %v", got, tc.want)
			}
		})
	}
}

// 폴더 생성과 그 안의 파일 생성이 폴더 이벤트 묶음 두 개로 나뉘어 한 번 기다리는 동안 모이면, lastPerPath가
// 파일의 C도 넘기는지 확인한다. 폴더의 C를 받은 hook이 그 뒤에 생긴 파일을 놓치지 않아야 한다.
func TestLastPerPathKeepsCreatesUnderCreatedFolder(t *testing.T) {
	f := newHookBatchFixture(t, t.TempDir(), hookBatchOption(300, 2000, domain.DeliverLastPerPath))
	mkdir(t, f.path("d"))
	f.add(fsnotify.Create, "d/", false)
	writeFile(t, f.path("d/f.txt"))
	f.add(fsnotify.Create, "d/f.txt", false)
	if got, want := f.flushWhenDue(), []string{"C d/", "C d/f.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}
