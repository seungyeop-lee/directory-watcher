package service

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// dirBatchFixture는 감시 대상 root와, root의 변경을 받는 dirBatcher를 둔다. dirBatcher가 넘긴 변경은 기다리지 않고
// 변경마다 실행을 만드는 hookBatcher가 받는다. 감시 루프 없이 handleEvent가 넘기는 변경을 add로 직접 넣는다.
type dirBatchFixture struct {
	t    *testing.T
	root string
	b    *dirBatcher
	// reAdded는 dirBatcher가 감시 목록에 다시 넣으라고 한 폴더의 상대 경로다. 그 폴더가 reAddFails이면
	// errReAdd로 실패한다.
	reAdded    []string
	reAddFails string
}

var errReAdd = errors.New("re-add failed")

func newDirBatchFixture(t *testing.T, option domain.DirBatchOption) *dirBatchFixture {
	f := &dirBatchFixture{t: t, root: t.TempDir()}
	hookBatcher := newHookBatcher(watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}}))
	f.b = newDirBatcher(option, hookBatcher, func(dir domain.Path) error {
		rel := f.rel(dir)
		f.reAdded = append(f.reAdded, rel)
		if rel == f.reAddFails {
			return errReAdd
		}
		return nil
	})
	f.b.root = f.root
	return f
}

func (f *dirBatchFixture) rel(path domain.Path) string {
	f.t.Helper()
	rel, err := filepath.Rel(f.root, path.String())
	if err != nil {
		f.t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}

// add는 감시 대상 기준 상대 경로 rel의 변경 op를 dirBatcher에 넣고, 곧바로 나온 실행을 describeRun의 꼴로 돌려준다.
// rel이 /로 끝나면 폴더의 변경이다.
func (f *dirBatchFixture) add(op fsnotify.Op, rel string) []string {
	f.t.Helper()
	runs, err := f.b.add(dirBatchChange(f.root, op, rel))
	if err != nil {
		f.t.Fatalf("add(%v %s) failed: %v", op, rel, err)
	}
	return f.describe(runs)
}

// describe는 실행 runs를 describeRun의 꼴로 돌려준다.
func (f *dirBatchFixture) describe(runs []domain.Event) []string {
	f.t.Helper()
	got := []string{}
	for _, run := range runs {
		got = append(got, describeRun(f.t, f.root, run))
	}
	return got
}

// awaitDue는 묶음을 넘길 때라고 Due가 알릴 때까지 기다린다.
func (f *dirBatchFixture) awaitDue() {
	f.t.Helper()
	select {
	case <-f.b.Due():
	case <-time.After(5 * time.Second):
		f.t.Fatal("dir batch did not become due")
	}
}

// dirBatchChange는 root 기준 상대 경로 rel의 변경 op를 만든다. rel이 /로 끝나면 폴더의 변경이다.
func dirBatchChange(root string, op fsnotify.Op, rel string) change {
	path := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
	return change{Event: domain.Event{Path: domain.Path(path), Operation: domain.Operation(op), IsDir: strings.HasSuffix(rel, "/")}}
}

// 폴더 이벤트를 모으는 동안 받은 변경 가운데 폴더 이벤트에 흡수되는 변경을 빼는지 확인한다. 폴더를 통째로 지우면
// 폴더 안의 항목을 지운 이벤트가 먼저 와도 폴더의 D 하나만 남는다.
func TestEventBatchAbsorbsEventsUnderFolderEvent(t *testing.T) {
	cases := []struct {
		name string
		ops  []fsnotify.Op
		rels []string
		want []string
	}{
		{
			name: "rm -rf nested folders",
			ops:  []fsnotify.Op{fsnotify.Remove, fsnotify.Remove, fsnotify.Remove, fsnotify.Remove},
			rels: []string{"pkg/f.txt", "pkg/sub/g.txt", "pkg/sub/", "pkg/"},
			want: []string{"D pkg/"},
		},
		{
			name: "mkdir -p and write a file in it",
			ops:  []fsnotify.Op{fsnotify.Create, fsnotify.Create, fsnotify.Create},
			rels: []string{"a/", "a/b/", "a/b/c.txt"},
			want: []string{"C a/"},
		},
		{
			name: "remove and write repeated for a removed folder",
			ops:  []fsnotify.Op{fsnotify.Remove, fsnotify.Remove, fsnotify.Write},
			rels: []string{"d/", "d", "d"},
			want: []string{"D d/"},
		},
		{
			name: "rm -rf and mkdir again",
			ops:  []fsnotify.Op{fsnotify.Remove, fsnotify.Create},
			rels: []string{"d/", "d/"},
			want: []string{"D d/", "C d/"},
		},
		{
			name: "rename",
			ops:  []fsnotify.Op{fsnotify.Remove, fsnotify.Create},
			rels: []string{"d/", "e/"},
			want: []string{"D d/", "C e/"},
		},
		{
			name: "file next to the folder",
			ops:  []fsnotify.Op{fsnotify.Create, fsnotify.Create, fsnotify.Write},
			rels: []string{"n/", "n/in.txt", "nx.txt"},
			want: []string{"C n/", "U nx.txt"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			var batch eventBatch
			for i, rel := range tc.rels {
				batch.add(dirBatchChange(root, tc.ops[i], rel))
			}
			got := []string{}
			for _, c := range batch.events {
				got = append(got, describeRun(t, root, c.Event))
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("batch = %v, want %v", got, tc.want)
			}
		})
	}
}

// isUnder가 dir과 같은 경로와, 이름이 dir로 시작하는 옆 경로를 dir 아래로 보지 않는지 확인한다.
func TestIsUnder(t *testing.T) {
	root := t.TempDir()
	dir := domain.Path(filepath.Join(root, "d"))
	cases := []struct {
		rel  string
		want bool
	}{
		{"d", false},
		{"d/a.txt", true},
		{"d/sub/a.txt", true},
		{"dx/a.txt", false},
		{"a.txt", false},
	}
	for _, tc := range cases {
		if got := isUnder(domain.Path(filepath.Join(root, filepath.FromSlash(tc.rel))), dir); got != tc.want {
			t.Errorf("isUnder(%s, d) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

// 폴더의 생성과 삭제, 감시 대상 바로 아래가 아닌 파일의 삭제로만 폴더 이벤트를 모으기 시작하는지 확인한다. 모으기를
// 시작하지 않는 변경은 곧바로 실행이 된다.
func TestDirBatchStartsOnFolderEventOrNestedRemove(t *testing.T) {
	cases := []struct {
		op    fsnotify.Op
		rel   string
		opens bool
	}{
		{fsnotify.Create, "d/", true},
		{fsnotify.Remove, "d/", true},
		{fsnotify.Remove, "d/a.txt", true},
		{fsnotify.Remove, "a.txt", false},
		{fsnotify.Create, "d/a.txt", false},
		{fsnotify.Write, "d/a.txt", false},
	}
	for _, tc := range cases {
		f := newDirBatchFixture(t, domain.DirBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: 1000})
		got := f.add(tc.op, tc.rel)
		if opens := len(got) == 0 && f.b.Due() != nil; opens != tc.opens {
			t.Errorf("%v %s: started batch = %v, want %v (runs %v)", tc.op, tc.rel, opens, tc.opens, got)
		}
	}
}

// wait가 0이면 폴더 이벤트를 모으지 않고 받은 변경을 곧바로 실행으로 돌려주는지 확인한다.
func TestDirBatchWithZeroWaitPassesChangesOn(t *testing.T) {
	f := newDirBatchFixture(t, domain.DirBatchOption{MaxWaitMillisecond: 1000})
	if got, want := f.add(fsnotify.Create, "d/"), []string{"C d/"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	if got, want := f.add(fsnotify.Create, "d/a.txt"), []string{"C d/a.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	if f.b.Due() != nil {
		t.Fatal("Due is set although wait is 0")
	}
}

// deliver: each에서 폴더 이벤트를 모으는 동안 생긴 다른 파일의 이벤트는 폴더 이벤트와 생긴 순서대로 실행이 되는지
// 확인한다.
func TestFolderEventKeepsOrderWithFileEvents(t *testing.T) {
	f := newDirBatchFixture(t, domain.DirBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: 1000})
	got := f.add(fsnotify.Create, "first.txt")
	for _, rel := range []string{"src/n/", "src/n/in.txt", "last.txt"} {
		if runs := f.add(fsnotify.Create, rel); len(runs) != 0 {
			t.Fatalf("add(%s) returned runs %v while collecting", rel, runs)
		}
	}
	f.awaitDue()
	runs, err := f.b.flush()
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, f.describe(runs)...)
	if want := []string{"C first.txt", "C src/n/", "C last.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}

// 새 폴더에 파일을 계속 써서 폴더 이벤트를 모으는 시간이 끝나지 않아도, dirBatch.maxWaitMillisecond 안에 폴더 C가
// 나가고 그 뒤의 쓰기는 따로 실행이 되는지 확인한다.
func TestFolderEventIsSentWithinMaxWaitDuringContinuousWrites(t *testing.T) {
	const maxWait = 500 * time.Millisecond
	f := newDirBatchFixture(t, domain.DirBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: domain.Millisecond(maxWait / time.Millisecond)})
	start := time.Now()
	f.add(fsnotify.Create, "src/n/")
	deadline := time.After(5 * time.Second)
	for due := false; !due; {
		select {
		case <-f.b.Due():
			due = true
		case <-time.After(50 * time.Millisecond):
			f.add(fsnotify.Write, "src/n/a.txt")
		case <-deadline:
			t.Fatal("dir batch did not become due while writes kept coming")
		}
	}
	// 느린 환경에서 타이머가 늦게 알리는 만큼 여유를 둔다.
	if elapsed := time.Since(start); elapsed > maxWait+300*time.Millisecond {
		t.Errorf("C of src/n was passed on after %v, want within %v", elapsed, maxWait)
	}
	runs, err := f.b.flush()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.describe(runs), []string{"C src/n/"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	if got, want := f.add(fsnotify.Write, "src/n/a.txt"), []string{"U src/n/a.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs after the folder event = %v, want %v", got, want)
	}
	if got, want := f.add(fsnotify.Write, "other.txt"), []string{"U other.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs after the folder event = %v, want %v", got, want)
	}
}

// kqueue에서는 생긴 폴더의 변경을 넘기기 전에 그 폴더 아래의 감시 폴더를 다시 넣게 하고, 다시 넣지 못하면 그 앞의
// 변경으로 만든 실행과 오류를 돌려주는지 확인한다. 다른 운영체제에서는 다시 넣지 않는다.
func TestCreatedFolderIsReAddedBeforePassingOn(t *testing.T) {
	f := newDirBatchFixture(t, domain.DirBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: 1000})
	f.reAddFails = "b"
	f.add(fsnotify.Create, "a/")
	f.add(fsnotify.Remove, "d/")
	f.add(fsnotify.Create, "b/")
	f.add(fsnotify.Create, "c/")
	f.awaitDue()
	runs, err := f.b.flush()
	wantReAdded, wantRuns, wantErr := []string{}, []string{"C a/", "D d/", "C b/", "C c/"}, error(nil)
	if kqueueOS {
		wantReAdded, wantRuns, wantErr = []string{"a", "b"}, []string{"C a/", "D d/"}, errReAdd
	}
	if got := append([]string{}, f.reAdded...); !slices.Equal(got, wantReAdded) {
		t.Errorf("re-added folders = %v, want %v", got, wantReAdded)
	}
	if got := f.describe(runs); !slices.Equal(got, wantRuns) {
		t.Errorf("runs = %v, want %v", got, wantRuns)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("flush error = %v, want %v", err, wantErr)
	}
}
