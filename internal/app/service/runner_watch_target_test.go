package service

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// controlledFixture는 fsnotify 이벤트를 직접 넣는 감시 대상 runner와, hook이 받은 실행을 둔다. 실제 watcher의
// 이벤트는 받지 않으므로, 디스크를 바꾼 뒤 그 이벤트를 send로 넣는다.
type controlledFixture struct {
	t      *testing.T
	root   string
	events chan<- fsnotify.Event
	runs   <-chan domain.Event
	result <-chan error
}

// startControlledRunner는 root를 option으로 감시하는 runner를 시작한다. onChange는 hook이 실행을 받을 때 먼저 실행한다.
func startControlledRunner(t *testing.T, root string, option domain.WatchTargetsOption, onChange func(domain.Event)) *controlledFixture {
	t.Helper()
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: option}, newMockLogger())
	events, _ := controlledSource(r)
	runs, result := startRunner(t, r, onChange)
	return &controlledFixture{t: t, root: root, events: events, runs: runs, result: result}
}

func (f *controlledFixture) path(rel string) string {
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

// send는 감시 대상 기준 상대 경로 rel의 fsnotify 이벤트 op를 감시 루프에 넣는다.
func (f *controlledFixture) send(op fsnotify.Op, rel string) {
	f.t.Helper()
	select {
	case f.events <- fsnotify.Event{Name: f.path(rel), Op: op}:
	case err := <-f.result:
		f.t.Fatalf("run stopped: %v", err)
	}
}

// collect는 hook이 quiet 동안 실행을 더 받지 않을 때까지 받은 실행을 describeRun의 꼴로 돌려준다.
func (f *controlledFixture) collect(quiet time.Duration) []string {
	f.t.Helper()
	got := []string{}
	for {
		select {
		case run := <-f.runs:
			got = append(got, describeRun(f.t, f.root, run))
		case err := <-f.result:
			f.t.Fatalf("run stopped: %v", err)
		case <-time.After(quiet):
			return got
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
