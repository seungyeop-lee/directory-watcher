package service

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

// queueRecorder는 대기열이 알린 단계 변화를 "70 up 70"처럼 단계, 방향, 기다리는 수로 모은다.
type queueRecorder struct {
	warnings []string
}

func (r *queueRecorder) warn(w queueWarning) {
	r.warnings = append(r.warnings, fmt.Sprintf("%d %s %d", w.level, w.direction, w.length))
}

// fillQueue는 대기열 q의 기다리는 수가 length가 될 때까지 실행을 넣거나 꺼낸다.
func fillQueue(t *testing.T, q *runQueue, length int) {
	t.Helper()
	for q.length() < length {
		if err := q.push(domain.Event{Path: "a.txt"}); err != nil {
			t.Fatal(err)
		}
	}
	for q.length() > length {
		q.pop()
	}
}

func (q *runQueue) length() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.runs)
}

// 기다리는 수가 0에서 용량 직전까지 올랐다가 0으로 내려오면, 70·90 올라감과 90·70 내려감을 한 번씩 알린다.
// 내려갈 때는 90은 80 아래에서, 70은 60 아래에서 알린다.
func TestQueueWarnsEachLevelOnceOnRiseAndFall(t *testing.T) {
	recorder := &queueRecorder{}
	q := newRunQueue(100, recorder.warn)
	fillQueue(t, q, 99)
	fillQueue(t, q, 0)
	if want := []string{"70 up 70", "90 up 90", "90 down 79", "70 down 59"}; !slices.Equal(recorder.warnings, want) {
		t.Fatalf("warnings = %q, want %q", recorder.warnings, want)
	}
}

// 70 경계에서 오르내려도 60 아래로 내려갔다가 다시 70에 닿을 때만 다시 알린다.
func TestQueueRewarnsOnlyAfterFallingBelowRelease(t *testing.T) {
	recorder := &queueRecorder{}
	q := newRunQueue(100, recorder.warn)
	for _, length := range []int{70, 65, 70, 60, 75, 69, 59, 69, 70} {
		fillQueue(t, q, length)
	}
	if want := []string{"70 up 70", "70 down 59", "70 up 70"}; !slices.Equal(recorder.warnings, want) {
		t.Fatalf("warnings = %q, want %q", recorder.warnings, want)
	}
}

// 용량이 작아 실행 하나로 두 단계를 넘으면 두 단계를 차례로 알린다.
func TestQueueWarnsEveryLevelPassedAtOnce(t *testing.T) {
	recorder := &queueRecorder{}
	q := newRunQueue(1, recorder.warn)
	fillQueue(t, q, 1)
	fillQueue(t, q, 0)
	if want := []string{"70 up 1", "90 up 1", "90 down 0", "70 down 0"}; !slices.Equal(recorder.warnings, want) {
		t.Fatalf("warnings = %q, want %q", recorder.warnings, want)
	}
}

func TestQueueFullRejectsRun(t *testing.T) {
	q := newRunQueue(2, func(queueWarning) {})
	fillQueue(t, q, 2)
	err := q.push(domain.Event{Path: "b.txt"})
	if !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full (capacity 2)") {
		t.Fatalf("push error = %v, want dispatch queue full (capacity 2)", err)
	}
}

// 경고에는 대기열에서 실행이 많은 경로부터 다섯 개까지 담기고, 수가 같으면 먼저 넣은 경로가 앞선다.
func TestQueueWarningListsMostQueuedPaths(t *testing.T) {
	var got []queuedPath
	q := newRunQueue(10, func(w queueWarning) { got = w.topPaths })
	for _, name := range []string{"f", "b", "a", "b", "c", "d", "e"} {
		if err := q.push(domain.Event{Path: domain.Path(name)}); err != nil {
			t.Fatal(err)
		}
	}
	want := []queuedPath{{"b", 2}, {"f", 1}, {"a", 1}, {"c", 1}, {"d", 1}}
	if !slices.Equal(got, want) {
		t.Fatalf("top paths = %v, want %v", got, want)
	}
}

// hookEnvironment는 ctx로 명령을 실행할 때 명령에 더하는 환경 변수를 돌려준다.
func hookEnvironment(ctx *helper.WatcherContext) map[string]string {
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

// 경고 hook이 끝나지 않은 동안에도 감시 루프는 이벤트를 받아 hook을 실행하고, 그동안 생긴 경고는 가장 최근 것
// 하나만 경고 hook이 끝난 뒤 실행한다.
func TestSlowQueueWarningKeepsOnlyLatestAndDoesNotBlockWatching(t *testing.T) {
	root := t.TempDir()
	warned, releaseWarning := make(chan struct{}, 8), make(chan struct{})
	var mu sync.Mutex
	var warnings []string
	global := domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnQueueWarning: runnerCommand(func(ctx *helper.WatcherContext, _ *domain.Event) error {
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
	r := NewWatchTargetRunner(global, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: option}, newMockLogger())
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, releaseChange, changed := make(chan struct{}), make(chan struct{}), make(chan string, 16)
	r.runStartHook = func() {}
	r.runFinishHook = func() {}
	r.runChangeHook = func(e domain.Event) {
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

// queue.size를 넘는 실행은 deliver 값마다 감시를 멈추고 onError를 한 번 실행한다.
func TestQueueSizeOverflowStopsWatchingAndNotifies(t *testing.T) {
	for name, deliver := range map[string]domain.Deliver{"last": domain.DeliverLast, "lastPerPath": domain.DeliverLastPerPath, "each": domain.DeliverEach} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			set := testCommandSet(root)
			set.WatchTargets[0].Option = hookBatchOption(0, 1000, deliver)
			set.WatchTargets[0].Option.Queue.Size = 4
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			set.WatchTargets[0].LifeCycle.OnChange = runnerCommand(func(ctx *helper.WatcherContext, _ *domain.Event) error {
				once.Do(func() { close(entered) })
				select {
				case <-release:
				case <-ctx.Context().Done():
				}
				return nil
			})
			notified := make(chan error, 2)
			set.Global.LifeCycle.OnError = runnerCommand(func(ctx *helper.WatcherContext, _ *domain.Event) error {
				notified <- errors.New(hookEnvironment(ctx)["DW_ERROR_CAUSE"])
				return nil
			})
			r := NewRunner(set, &notificationLog{})
			assertWatchDirectoriesRemovable(t, r.watchTargets...)
			defer close(release)
			events, _ := controlledSource(r.watchTargets[0])
			go r.Run()
			send := func(name string) {
				path := filepath.Join(root, name)
				writeFile(t, path)
				events <- fsnotify.Event{Name: path, Op: fsnotify.Create}
			}
			send("first")
			<-entered
			for n := range 5 {
				send(fmt.Sprint(n))
			}
			select {
			case err := <-r.Errors():
				if !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full (capacity 4)") {
					t.Fatalf("fatal error = %v, want dispatch queue full (capacity 4)", err)
				}
			case <-time.After(time.Second):
				t.Fatal("queue overflow did not stop watching")
			}
			r.Stop()
			if len(notified) != 1 {
				t.Fatalf("onError ran %d times, want 1", len(notified))
			}
			if cause := <-notified; !strings.Contains(cause.Error(), "dispatch queue full (capacity 4)") {
				t.Fatalf("DW_ERROR_CAUSE = %q", cause)
			}
		})
	}
}
