package service

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

// testBinaryCommand는 이 테스트 바이너리를 flags로 다시 실행하는 hook 명령이다. 인자를 큰따옴표로 감싸
// 기본 셸(sh, cmd)이 경로의 공백이나 -test.run의 ^, $를 해석하지 않게 한다.
// -race로 빌드한 바이너리는 종료 코드 0으로 끝날 때 1초를 기다리므로, GORACE의 atexit_sleep_ms=0을 hook에 넘긴다.
func testBinaryCommand(t *testing.T, flags string) string {
	t.Helper()
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	return fmt.Sprintf(`"%s" "%s"`, os.Args[0], flags)
}

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

// setDirRemoveRepeat은 테스트 동안 지운 폴더의 REMOVE가 한 번 더 올 수 있는 운영체제인지를 repeat로 정한다.
func setDirRemoveRepeat(t *testing.T, repeat bool) {
	t.Helper()
	saved := dirRemoveRepeatOS
	dirRemoveRepeatOS = repeat
	t.Cleanup(func() { dirRemoveRepeatOS = saved })
}

// startRunner는 r의 dispatcher가 실행하는 hook 세 개를 바꿔 Run을 시작하고, 감시 목록 등록이 끝나 start hook이
// 불리면 돌려준다. change hook은 onChange가 있으면 먼저 실행하고 받은 실행을 changed로 보낸다. result는 Run의 결과를
// 받는다. 테스트가 끝나면 r을 멈춘다.
func startRunner(t *testing.T, r *watchTargetRunner, onChange func(domain.Event)) (changed <-chan domain.Event, result <-chan error) {
	t.Helper()
	t.Cleanup(func() {
		var wg sync.WaitGroup
		wg.Add(1)
		r.Stop(&wg)
	})
	ready, events, done := make(chan struct{}), make(chan domain.Event, testQueueSize), make(chan error, 1)
	r.dispatcher.hooks.runStartHook = func() { close(ready) }
	r.dispatcher.hooks.runChangeHook = func(event domain.Event) {
		if onChange != nil {
			onChange(event)
		}
		events <- event
	}
	r.dispatcher.hooks.runFinishHook = func() {}
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
	r.tree.events, r.tree.errs = events, watchErrors
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

// hookCalls는 명령이 실행될 때 받은 context의 hook 이름과 명령 실행 방식을 실행한 순서대로 모은다.
type hookCalls struct {
	mu    sync.Mutex
	calls []string
}

// command는 실행될 때 "start onStartWatch {Silent:false DryRun:true}"처럼 label, hook 이름, 명령 실행 방식을 기록하는 명령이다.
func (c *hookCalls) command(label string) domain.Cmd {
	return runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.calls = append(c.calls, fmt.Sprintf("%s %s %+v", label, ctx.Hook(), ctx.Mode()))
		return nil
	})
}

func (c *hookCalls) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
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

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
}

func removeAll(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}

// describeRun은 hook이 받는 실행 run을 root 기준 상대 경로로 "C a.txt"처럼 쓴다. 폴더는 "C d/"처럼 경로 끝에
// /를 붙인다.
func describeRun(t *testing.T, root string, run domain.Event) string {
	t.Helper()
	rel, err := filepath.Rel(root, run.Path.String())
	if err != nil {
		t.Fatal(err)
	}
	rel = filepath.ToSlash(rel)
	if run.IsDir {
		rel += "/"
	}
	return run.Operation.String() + " " + rel
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
	// holdDirBatch가 true이면 첫 changes의 표지 파일을 감시 루프가 모두 기록할 때까지 dirBatcher가 묶음을 넘기지
	// 않는다. holdDirBatchUntilSentinels를 본다. runner를 시작하기 전에 정한다.
	holdDirBatch bool
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
	f.runner = NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(f.target), Option: option}, newMockLogger(), process.CommandMode{})
	if f.holdDirBatch {
		f.runner.dirBatcher.after = f.holdDirBatchUntilSentinels()
	}
	f.changed, f.result = startRunner(f.t, f.runner, nil)
}

// holdDirBatchUntilSentinels는 dirBatcher의 after를 대신할 함수를 돌려준다. 이 함수는 감시 루프가 다음 changes의
// 표지 파일을 모두 기록하기 전에는 nil 채널을 돌려주어 dirBatcher가 묶음을 넘기지 않게 하고, 그 뒤로는 time.After와
// 같다. 그래서 동작이 maxWaitMillisecond보다 오래 걸리거나 동작 중에 이벤트 사이가 waitMillisecond보다 벌어져도,
// 동작의 이벤트는 표지 파일의 이벤트와 한 묶음에 모인다. Windows CI에서는 파일 300개가 든 폴더를 지우는 동안
// 이벤트 사이가 100ms를 넘어 묶음이 나뉜 적이 있다.
//
// dirBatcher는 감시 루프의 고루틴에서 after를 부르므로, 돌려준 함수가 기록을 읽어도 감시 루프와 겹치지 않는다.
func (f *renameFixture) holdDirBatchUntilSentinels() func(time.Duration) <-chan time.Time {
	tree := f.runner.tree
	var sentinels []string
	for _, rel := range f.sentinels(f.phase + 1) {
		sentinels = append(sentinels, f.path(rel))
	}
	released := false
	return func(d time.Duration) <-chan time.Time {
		released = released || !slices.ContainsFunc(sentinels, func(p string) bool {
			_, ok := tree.recorded(p)
			return !ok
		})
		if !released {
			return nil
		}
		return time.After(d)
	}
}

// sentinels는 phase번째 changes가 쓰는 표지 파일의 감시 대상 기준 상대 경로다.
func (f *renameFixture) sentinels(phase int) []string {
	var rels []string
	for _, dir := range f.sentinelDirs {
		rels = append(rels, filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("zz-sentinel-%d.txt", phase))))
	}
	return rels
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
	waiting := map[string]bool{}
	for _, rel := range f.sentinels(f.phase) {
		f.write(rel)
		waiting[rel] = true
	}
	var got []string
	for len(waiting) > 0 {
		ev := awaitHookEvent(f.t, f.changed, f.result)
		rel, err := filepath.Rel(f.target, ev.Path.String())
		if err != nil {
			f.t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(filepath.Base(rel), "zz-sentinel-") {
			delete(waiting, rel)
			continue
		}
		got = append(got, describeRun(f.t, f.target, ev))
	}
	return got
}

// next는 hook이 받은 다음 이벤트를 changes와 같은 꼴로 돌려준다.
func (f *renameFixture) next() string {
	f.t.Helper()
	return describeRun(f.t, f.target, awaitHookEvent(f.t, f.changed, f.result))
}

// assertNoWatchUnder는 감시 목록에 rel과 그 아래 경로가 없는지 확인한다.
//
// Windows에서는 옮겨 나간 폴더의 감시를 leakCheckDelay 뒤에 watcher를 새로 만들어 버리므로, 그보다 오래 기다린
// 뒤 표지 파일의 hook이 실행될 때까지 기다린다. 감시 루프는 이벤트와 watcher 교체를 차례로 처리하므로, 표지
// 파일의 hook이 실행됐다면 교체도 끝났고 runner.tree.watcher를 읽어도 된다.
func (f *renameFixture) assertNoWatchUnder(rel string) {
	f.t.Helper()
	time.Sleep(3 * leakCheckDelay)
	f.changes()
	for _, p := range f.runner.tree.watcher.WatchList() {
		if domain.Path(f.path(rel)).Contains(domain.Path(p)) {
			f.t.Errorf("watch list still holds %s under the old path %s", p, rel)
		}
	}
}

// assertRecorded는 감시 대상 기준 상대 경로 rel이 기록에 있는지와 폴더인지가 want, wantDir와 같은지 확인한다.
func (f *renameFixture) assertRecorded(rel string, want, wantDir bool) {
	f.t.Helper()
	assertRecorded(f.t, f.runner.tree, f.target, rel, want, wantDir)
}

// assertRecordMatchesDisk는 감시 대상 안에 있는 모든 항목이 기록에 같은 종류로 있고, gone의 경로는 기록에
// 없는지 확인한다. exclude와 subDir: false가 기록에서 빼는 항목이 없는 감시 대상에서만 쓴다.
func (f *renameFixture) assertRecordMatchesDisk(gone ...string) {
	f.t.Helper()
	assertRecordMatchesDisk(f.t, f.runner.tree, f.target, gone...)
}

// assertRecorded는 감시 대상 root 기준 상대 경로 rel이 tree의 기록에 있는지와 폴더인지가 want, wantDir와 같은지
// 확인한다.
func assertRecorded(t *testing.T, tree *watchTree, root, rel string, want, wantDir bool) {
	t.Helper()
	isDir, ok := tree.recorded(filepath.Join(root, filepath.FromSlash(rel)))
	switch {
	case ok != want:
		t.Errorf("recorded(%s) = %v, want %v", rel, ok, want)
	case ok && isDir != wantDir:
		t.Errorf("recorded(%s) reports folder %v, want %v", rel, isDir, wantDir)
	}
}

// assertRecordMatchesDisk는 감시 대상 root 안에 있는 모든 항목이 tree의 기록에 같은 종류로 있고, gone의 경로는
// 기록에 없는지 확인한다. exclude와 subDir: false가 기록에서 빼는 항목이 없는 감시 대상에서만 쓴다.
func assertRecordMatchesDisk(t *testing.T, tree *watchTree, root string, gone ...string) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		assertRecorded(t, tree, root, filepath.ToSlash(rel), true, d.IsDir())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range gone {
		assertRecorded(t, tree, root, rel, false, false)
	}
}

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
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: option}, newMockLogger(), process.CommandMode{})
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

func appendFile(t *testing.T, name string) {
	t.Helper()
	f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("y\n"); err != nil {
		t.Fatal(err)
	}
}

// waitPlainEvent는 plain.txt에서 op 이벤트가 hook에 도착할 때까지 기다리며, 그 사이에 기본 제외 대상의
// 이벤트로 hook이 실행되면 실패한다.
func waitPlainEvent(t *testing.T, changed <-chan domain.Event, op fsnotify.Op) {
	t.Helper()
	for {
		got := awaitHookEvent(t, changed, nil)
		if got.Path.IsDefaultExcludeFile() {
			t.Fatalf("%s of default-excluded name ran the hook: %s", got.Operation, got.Path)
		}
		if got.Path.Name() == "plain.txt" && fsnotify.Op(got.Operation).Has(op) {
			return
		}
	}
}

// awaitHookPath는 hook이 path의 이벤트를 받을 때까지 기다리며, 그 사이에 받은 이벤트의 경로를 돌려준다.
func awaitHookPath(t *testing.T, changed <-chan domain.Event, result <-chan error, path string) []string {
	t.Helper()
	var seen []string
	for {
		got := awaitHookEvent(t, changed, result)
		seen = append(seen, got.Path.String())
		if got.Path.String() == path {
			return seen
		}
	}
}

// eventLogger는 감시 루프가 받은 이벤트의 DEBUG 로그를 lines로 보낸다.
type eventLogger struct{ lines chan string }

func (l *eventLogger) Info(string)  {}
func (l *eventLogger) Error(string) {}
func (l *eventLogger) Debug(msg string) {
	if strings.HasPrefix(msg, "event: ") {
		l.lines <- msg
	}
}

// await는 name이 든 이벤트 로그가 올 때까지 기다린다.
func (l *eventLogger) await(t *testing.T, name string) {
	t.Helper()
	for {
		select {
		case line := <-l.lines:
			if strings.Contains(line, name) {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("watcher did not receive an event of %s", name)
		}
	}
}

// hookEnvironment는 ctx로 명령을 실행할 때 명령에 더하는 환경 변수를 돌려준다.
func hookEnvironment(ctx *process.WatcherContext) map[string]string {
	cmd := exec.Command("hook")
	cmd.Env = []string{}
	ctx.ApplyGracefulShutdown(cmd)
	env := map[string]string{}
	for _, kv := range cmd.Env {
		key, value, _ := strings.Cut(kv, "=")
		env[key] = value
	}
	return env
}

// awaitHookEvent는 hook이 받은 다음 이벤트를 기다린다. result가 nil이면 runner 종료는 기다리지 않는다.
func awaitHookEvent(t *testing.T, changed <-chan domain.Event, result <-chan error) domain.Event {
	t.Helper()
	select {
	case event := <-changed:
		return event
	case err := <-result:
		t.Fatalf("run stopped: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no event reached the hook")
	}
	return domain.Event{}
}
