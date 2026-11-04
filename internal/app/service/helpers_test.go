package service

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

// mockLogger는 테스트용 모의 Logger다.
type mockLogger struct{}

func (m *mockLogger) Info(msg string)  {}
func (m *mockLogger) Error(msg string) {}
func (m *mockLogger) Debug(msg string) {}

func newMockLogger() helper.Logger {
	return &mockLogger{}
}

type notificationLog struct {
	sync.Mutex
	messages []string
}

func (l *notificationLog) Debug(string) {}
func (l *notificationLog) Info(string)  {}
func (l *notificationLog) Error(s string) {
	l.Lock()
	defer l.Unlock()
	l.messages = append(l.messages, s)
}
func (l *notificationLog) String() string {
	l.Lock()
	defer l.Unlock()
	return strings.Join(l.messages, "\n")
}

// testQueueSize는 대기열을 채우지 않는 테스트의 대기열 용량이다. hook이 받은 실행을 모으는 채널도 이만큼 받아 둔다.
const testQueueSize = 2048

// watchTargetOption은 감시 옵션이 watch인 감시 대상 옵션을 만든다. 폴더 이벤트는 설정 기본값인 100ms와 1000ms로
// 모으고, hook에는 이벤트를 기다리지 않고 하나씩 넘기며, 대기열 용량은 testQueueSize다.
func watchTargetOption(watch domain.WatchOption) domain.WatchTargetsOption {
	return domain.WatchTargetsOption{
		Watch:     watch,
		DirBatch:  domain.DirBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: 1000},
		HookBatch: domain.HookBatchOption{Deliver: domain.DeliverEach},
		Queue:     domain.QueueOption{Size: testQueueSize},
	}
}

// hookBatchOption은 설정 파일에 option을 적지 않았을 때의 감시 대상 옵션에서 hookBatch 값만 바꾼다. 대기열 용량은
// testQueueSize다.
func hookBatchOption(wait, maxWait domain.Millisecond, deliver domain.Deliver) domain.WatchTargetsOption {
	option := watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}})
	option.HookBatch = domain.HookBatchOption{WaitMillisecond: wait, MaxWaitMillisecond: maxWait, Deliver: deliver}
	option.Run.Concurrency = 1
	return option
}

// startRunner는 r의 hook 세 개를 바꿔 Run을 시작하고, 감시 목록 등록이 끝나 start hook이 불리면 돌려준다.
// change hook은 onChange가 있으면 먼저 실행하고 받은 실행을 changed로 보낸다. result는 Run의 결과를 받는다.
// 테스트가 끝나면 r을 멈춘다.
func startRunner(t *testing.T, r *watchTargetRunner, onChange func(domain.Event)) (changed <-chan domain.Event, result <-chan error) {
	t.Helper()
	t.Cleanup(func() {
		var wg sync.WaitGroup
		wg.Add(1)
		r.Stop(&wg)
	})
	ready, events, done := make(chan struct{}), make(chan domain.Event, testQueueSize), make(chan error, 1)
	r.runStartHook = func() { close(ready) }
	r.runChangeHook = func(event domain.Event) {
		if onChange != nil {
			onChange(event)
		}
		events <- event
	}
	r.runFinishHook = func() {}
	go func() { done <- r.Run() }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("run before registration: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not become ready")
	}
	return events, done
}

// controlledSource는 r이 실제 watcher 대신, 돌려준 두 채널에서 이벤트와 오류를 받게 한다.
func controlledSource(r *watchTargetRunner) (chan fsnotify.Event, chan error) {
	events, watchErrors := make(chan fsnotify.Event), make(chan error)
	r.watchEvents, r.watchErrors = events, watchErrors
	return events, watchErrors
}

func assertWatchDirectoriesRemovable(t *testing.T, targets ...*watchTargetRunner) {
	t.Helper()
	t.Cleanup(func() {
		for _, target := range targets {
			before := time.Now()
			err := os.RemoveAll(target.path.String())
			t.Logf("resource removal path=%q at=%s duration=%s err=%v", target.path, time.Now().UTC().Format(time.RFC3339Nano), time.Since(before), err)
			if err != nil {
				t.Errorf("watch directory remains in use after Stop: %v", err)
			}
		}
	})
}

type runnerCommand func(*process.WatcherContext, *domain.Event) error

func (f runnerCommand) Run(ctx *process.WatcherContext, _ domain.Path, event *domain.Event) error {
	return f(ctx, event)
}
func emptyCommand() domain.Cmd {
	return runnerCommand(func(*process.WatcherContext, *domain.Event) error { return nil })
}

func testCommandSet(paths ...string) domain.CommandSet {
	empty := emptyCommand()
	set := domain.CommandSet{Global: domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnStartWatch: empty, OnBeforeChange: empty, OnAfterChange: empty, OnFinishWatch: empty}}}
	for _, path := range paths {
		option := watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})
		option.Run.Concurrency = 1
		set.WatchTargets = append(set.WatchTargets, domain.WatchTargetsCommandSet{Path: domain.Path(path), LifeCycle: domain.WatchTargetsLifeCycle{OnStartWatch: empty, OnChange: empty, OnFinishWatch: empty}, Option: option})
	}
	return set
}

// chdir는 작업 디렉터리를 dir로 옮기고 테스트가 끝나면 되돌린다. Windows는 작업 디렉터리로 쓰는 폴더를
// 지우지 못하므로, dir은 이 함수를 부르기 전에 만든 TempDir 안에 있어야 TempDir 정리보다 먼저 되돌려진다.
func chdir(t *testing.T, dir string) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatal(err)
		}
	})
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
}

// renameFixture는 감시 대상 target 안에 src와 dst 폴더를, target 밖에 outside 폴더를 둔다.
type renameFixture struct {
	t       *testing.T
	target  string
	outside string
	runner  *watchTargetRunner
	changed <-chan domain.Event
	result  <-chan error
	phase   int
	// sentinelDirs는 changes가 표지 파일을 쓰는 폴더다. 감시 대상 기준 상대 경로이고, ""는 감시 대상이다.
	sentinelDirs []string
}

func newRenameFixture(t *testing.T) *renameFixture {
	base := t.TempDir()
	f := &renameFixture{t: t, target: filepath.Join(base, "target"), outside: filepath.Join(base, "outside"), sentinelDirs: []string{"", "src", "dst"}}
	for _, dir := range []string{"target/src", "target/dst", "outside"} {
		if err := os.MkdirAll(filepath.Join(base, filepath.FromSlash(dir)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// path는 감시 대상 기준 상대 경로 rel을 경로로 바꾼다. "../outside/"로 시작하면 감시 대상 밖 경로다.
func (f *renameFixture) path(rel string) string {
	return filepath.Join(f.target, filepath.FromSlash(rel))
}

func (f *renameFixture) write(rel string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.path(rel)), 0700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.path(rel), []byte("event"), 0600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *renameFixture) move(from, to string) {
	f.t.Helper()
	if err := os.Rename(f.path(from), f.path(to)); err != nil {
		f.t.Fatal(err)
	}
}

// fill은 감시 대상 기준 상대 경로 dir 안에 파일 n개를 만든다.
func (f *renameFixture) fill(dir string, n int) {
	f.t.Helper()
	for i := 0; i < n; i++ {
		f.write(fmt.Sprintf("%s/f%d.txt", dir, i))
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

func (f *renameFixture) mkdirAll(rel string) {
	f.t.Helper()
	if err := os.MkdirAll(f.path(rel), 0700); err != nil {
		f.t.Fatal(err)
	}
}

func (f *renameFixture) removeAll(rel string) {
	f.t.Helper()
	if err := os.RemoveAll(f.path(rel)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *renameFixture) start(exclude domain.ExcludePatterns) {
	f.startWith(watchTargetOption(domain.WatchOption{SubDir: true, Exclude: exclude, FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}}))
}

func (f *renameFixture) startWith(option domain.WatchTargetsOption) {
	f.runner = NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(f.target), Option: option}, newMockLogger())
	f.changed, f.result = startRunner(f.t, f.runner, nil)
}

// stop은 runner를 멈추고 감시 루프가 끝날 때까지 기다린다. 그 뒤에는 기록을 읽어도 감시 루프와 겹치지 않는다.
func (f *renameFixture) stop() {
	f.t.Helper()
	var wg sync.WaitGroup
	wg.Add(1)
	f.runner.Stop(&wg)
}

// changes는 sentinelDirs의 폴더마다 표지 파일을 쓰고, 그 표지들의 hook이 모두 실행될 때까지 hook이 받은
// 이벤트를 "D src/a.txt"처럼 돌려준다. 폴더 이벤트는 "C src/d/"처럼 경로 끝에 /를 붙인다. 표지 파일의 이벤트는
// 빼고 돌려준다.
//
// Windows는 폴더마다 따로 이벤트를 받으므로, 한 폴더의 표지만 기다리면 다른 폴더의 이벤트가 그 뒤에 올 수 있다.
func (f *renameFixture) changes() []string {
	f.t.Helper()
	f.phase++
	sentinel := fmt.Sprintf("zz-sentinel-%d.txt", f.phase)
	waiting := map[string]bool{}
	for _, dir := range f.sentinelDirs {
		rel := filepath.ToSlash(filepath.Join(dir, sentinel))
		f.write(rel)
		waiting[rel] = true
	}
	var got []string
	for len(waiting) > 0 {
		select {
		case ev := <-f.changed:
			rel, err := filepath.Rel(f.target, ev.Path.String())
			if err != nil {
				f.t.Fatal(err)
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(filepath.Base(rel), "zz-sentinel-") {
				delete(waiting, rel)
				continue
			}
			if ev.IsDir {
				rel += "/"
			}
			got = append(got, ev.Operation.String()+" "+rel)
		case err := <-f.result:
			f.t.Fatalf("run stopped: %v", err)
		case <-time.After(5 * time.Second):
			f.t.Fatalf("sentinels %v did not reach the hook (got %v)", waiting, got)
		}
	}
	return got
}

// next는 hook이 받은 다음 이벤트를 changes와 같은 꼴로 돌려준다.
func (f *renameFixture) next() string {
	f.t.Helper()
	select {
	case ev := <-f.changed:
		rel, err := filepath.Rel(f.target, ev.Path.String())
		if err != nil {
			f.t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		if ev.IsDir {
			rel += "/"
		}
		return ev.Operation.String() + " " + rel
	case err := <-f.result:
		f.t.Fatalf("run stopped: %v", err)
	case <-time.After(5 * time.Second):
		f.t.Fatal("no event reached the hook")
	}
	return ""
}

// assertNoWatchUnder는 감시 목록에 rel과 그 아래 경로가 없는지 확인한다.
//
// Windows에서는 옮겨 나간 폴더의 감시를 leakCheckDelay 뒤에 watcher를 새로 만들어 버리므로, 그보다 오래 기다린
// 뒤 표지 파일의 hook이 실행될 때까지 기다린다. 감시 루프는 이벤트와 watcher 교체를 차례로 처리하므로, 표지
// 파일의 hook이 실행됐다면 교체도 끝났고 runner.watcher를 읽어도 된다.
func (f *renameFixture) assertNoWatchUnder(rel string) {
	f.t.Helper()
	time.Sleep(3 * leakCheckDelay)
	f.changes()
	for _, p := range f.runner.watcher.WatchList() {
		if domain.Path(f.path(rel)).Contains(domain.Path(p)) {
			f.t.Errorf("watch list still holds %s under the old path %s", p, rel)
		}
	}
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
// 없는지 확인한다. exclude와 subDir: false가 기록에서 빼는 항목이 없는 감시 대상에서만 쓴다.
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
