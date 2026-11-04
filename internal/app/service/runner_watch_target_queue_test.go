package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
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

func TestDispatcherRunTerminatesWhenStopCloses(t *testing.T) {
	stop := make(chan struct{})
	d := newDispatcher(hookFunc{runChangeHook: func(domain.Event) {}}, testQueueSize, nil, stop)
	done := make(chan struct{})
	go func() { d.run(); close(done) }()
	close(stop)
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stop leaked the run loop waiting on an empty queue")
	}
}

func TestQueueWarningHookRunsWithItsHookNameAndCommandMode(t *testing.T) {
	calls := &hookCalls{}
	qw := &queueWarner{hook: calls.command("warning"), target: "target", logger: newMockLogger(), mode: process.CommandMode{DryRun: true}}
	qw.notify(queueWarning{level: 70, direction: "up", length: 7, capacity: 10})
	qw.stop()
	if got, want := calls.list(), []string{"warning onQueueWarning {Silent:false DryRun:true}"}; !slices.Equal(got, want) {
		t.Fatalf("hook calls = %q, want %q", got, want)
	}
}
