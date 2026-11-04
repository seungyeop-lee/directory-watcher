package service

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

// queueLevel은 대기열이 찬 정도의 단계다. 경고 단계의 값은 그 단계에 들어서는 백분율이다.
type queueLevel int

const (
	levelNormal  queueLevel = 0
	levelWarning queueLevel = 70
	levelDanger  queueLevel = 90
)

// levelChanges는 단계를 바꾸는 규칙이다. from 단계에서 기다리는 수가 용량의 percent%에 닿거나(rising) percent%
// 아래로 내려가면 to 단계가 된다. 내려갈 때의 경계를 올라갈 때보다 10%p 낮춰, 경계 근처에서 같은 경고가 반복되지
// 않게 한다.
var levelChanges = []struct {
	from, to queueLevel
	percent  int
	rising   bool
}{
	{levelNormal, levelWarning, 70, true},
	{levelWarning, levelDanger, 90, true},
	{levelDanger, levelWarning, 80, false},
	{levelWarning, levelNormal, 60, false},
}

// topPathCount는 경고에 담는 경로의 최대 수다.
const topPathCount = 5

// queueWarning은 대기열의 단계 변화다. level은 바뀐 두 단계 가운데 경고 단계이고, direction은 up이나 down이다.
// length는 단계가 바뀐 때 기다리는 실행의 수다.
type queueWarning struct {
	level     queueLevel
	direction string
	length    int
	capacity  int
	topPaths  []queuedPath
}

// queuedPath는 대기열에서 경로 path의 실행이 count개 기다린다는 뜻이다.
type queuedPath struct {
	path  domain.Path
	count int
}

// runQueue는 감시 대상에서 hook 실행을 기다리는 실행의 대기열이다. 용량을 넘는 실행은 받지 않고, 실행을 넣고
// 꺼낼 때마다 기다리는 수로 단계를 계산해 단계가 바뀌면 warn으로 알린다.
type runQueue struct {
	mu       sync.Mutex
	runs     []domain.Event
	capacity int
	level    queueLevel
	warn     func(queueWarning)
	// pushed는 대기열에 실행을 넣었음을 pop에 알린다.
	pushed chan struct{}
}

func newRunQueue(capacity int, warn func(queueWarning)) *runQueue {
	return &runQueue{capacity: capacity, warn: warn, pushed: make(chan struct{}, 1)}
}

// push는 run을 대기열 끝에 넣는다. 대기열이 가득 찼으면 기다리지 않고 오류를 돌려준다.
func (q *runQueue) push(run domain.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.runs) == q.capacity {
		return fmt.Errorf("dispatch queue full (capacity %d): %w", q.capacity, fsnotify.ErrEventOverflow)
	}
	q.runs = append(q.runs, run)
	q.updateLevel()
	select {
	case q.pushed <- struct{}{}:
	default:
	}
	return nil
}

// first는 맨 앞의 실행을 꺼내지 않고 돌려준다. 대기열이 비었으면 실행이 들어오거나 stop이 닫힐 때까지 기다리고,
// stop이 닫히면 false를 돌려준다.
func (q *runQueue) first(stop <-chan struct{}) (domain.Event, bool) {
	for {
		q.mu.Lock()
		if len(q.runs) > 0 {
			run := q.runs[0]
			q.mu.Unlock()
			return run, true
		}
		q.mu.Unlock()
		select {
		case <-stop:
			return domain.Event{}, false
		case <-q.pushed:
		}
	}
}

// pop은 맨 앞의 실행을 뺀다. 대기열에서 실행을 빼는 쪽은 dispatcher.run 하나뿐이므로, first가 돌려준 실행을 뺀다.
func (q *runQueue) pop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.runs = q.runs[1:]
	q.updateLevel()
}

// updateLevel은 기다리는 수로 단계를 바꾸고, 바꿀 때마다 warn으로 알린다. 용량이 작아 실행 하나로 두 경계를
// 넘으면 두 번 알린다. q.mu를 잡고 부른다.
func (q *runQueue) updateLevel() {
	for changed := true; changed; {
		changed = false
		for _, c := range levelChanges {
			reached := len(q.runs)*100 >= c.percent*q.capacity
			if c.from != q.level || reached != c.rising {
				continue
			}
			q.level, changed = c.to, true
			direction := "down"
			if c.rising {
				direction = "up"
			}
			q.warn(queueWarning{level: max(c.from, c.to), direction: direction, length: len(q.runs), capacity: q.capacity, topPaths: q.topPaths()})
			break
		}
	}
}

// topPaths는 대기열에서 실행이 많은 경로부터 topPathCount개까지 돌려준다. 수가 같으면 먼저 넣은 경로가 앞선다.
func (q *runQueue) topPaths() []queuedPath {
	var paths []queuedPath
	index := map[domain.Path]int{}
	for _, run := range q.runs {
		i, ok := index[run.Path]
		if !ok {
			i = len(paths)
			index[run.Path] = i
			paths = append(paths, queuedPath{path: run.Path})
		}
		paths[i].count++
	}
	slices.SortStableFunc(paths, func(a, b queuedPath) int { return b.count - a.count })
	return paths[:min(len(paths), topPathCount)]
}

// queueWarningVariables는 onQueueWarning hook에 대기열 경고의 정보를 넘기는 환경 변수다.
var queueWarningVariables = []struct {
	name  string
	value func(target domain.Path, w queueWarning) string
}{
	{"DW_QUEUE_TARGET", func(target domain.Path, _ queueWarning) string { return target.String() }},
	{"DW_QUEUE_LEVEL", func(_ domain.Path, w queueWarning) string { return strconv.Itoa(int(w.level)) }},
	{"DW_QUEUE_DIRECTION", func(_ domain.Path, w queueWarning) string { return w.direction }},
	{"DW_QUEUE_LENGTH", func(_ domain.Path, w queueWarning) string { return strconv.Itoa(w.length) }},
	{"DW_QUEUE_CAPACITY", func(_ domain.Path, w queueWarning) string { return strconv.Itoa(w.capacity) }},
	{"DW_QUEUE_TOP_PATHS", func(_ domain.Path, w queueWarning) string {
		lines := make([]string, len(w.topPaths))
		for i, p := range w.topPaths {
			lines[i] = fmt.Sprintf("%d %s", p.count, p.path)
		}
		return strings.Join(lines, "\n")
	}},
}

// QueueWarningVariableNames는 onQueueWarning hook이 받는 환경 변수의 이름을 돌려준다.
func QueueWarningVariableNames() []string {
	names := make([]string, len(queueWarningVariables))
	for i, v := range queueWarningVariables {
		names[i] = v.name
	}
	return names
}

// queueWarner는 감시 대상 target의 대기열 경고로 onQueueWarning hook을 감시 루프와 따로 실행한다. hook이 실행
// 중일 때 생긴 경고는 가장 최근 것 하나만 남겼다가, 지금 실행이 끝나면 실행한다.
type queueWarner struct {
	hook   domain.Cmd
	target domain.Path
	logger helper.Logger
	mode   process.CommandMode

	mu      sync.Mutex
	running bool
	pending *queueWarning
	stopped bool
	idle    sync.WaitGroup
}

// notify는 w로 hook을 실행하고 기다리지 않고 돌아온다.
func (qw *queueWarner) notify(w queueWarning) {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	if qw.stopped {
		return
	}
	if qw.running {
		qw.pending = &w
		return
	}
	qw.running = true
	qw.idle.Add(1)
	go qw.run(w)
}

func (qw *queueWarner) run(w queueWarning) {
	defer qw.idle.Done()
	for {
		qw.runHook(w)
		qw.mu.Lock()
		if qw.pending == nil {
			qw.running = false
			qw.mu.Unlock()
			return
		}
		w, qw.pending = *qw.pending, nil
		qw.mu.Unlock()
	}
}

// runHook은 onError처럼 정해진 시간 안에 hook을 실행하고, 실패하면 기록한다.
func (qw *queueWarner) runHook(w queueWarning) {
	base := process.NewWatcherContext("[QUEUE WARNING HOOK]", qw.mode)
	defer base.Cancel()
	var env []string
	for _, v := range queueWarningVariables {
		env = append(env, v.name+"="+v.value(qw.target, w))
	}
	ctx := base.NewTimeoutRun(notificationTimeout, env)
	defer ctx.Cancel()
	if err := qw.hook.Run(ctx.WithHook("onQueueWarning"), "", nil); err != nil {
		if ctx.Context().Err() != nil {
			qw.logger.Error(fmt.Sprintf("onQueueWarning notification failed: %v: %v", ctx.Context().Err(), err))
		} else {
			qw.logger.Error(fmt.Sprintf("onQueueWarning notification failed: %v", err))
		}
	}
}

// stop은 남겨 둔 경고를 버리고, 실행 중인 hook이 끝날 때까지 기다린다. stop 뒤의 경고는 실행하지 않는다.
func (qw *queueWarner) stop() {
	qw.mu.Lock()
	qw.stopped, qw.pending = true, nil
	qw.mu.Unlock()
	qw.idle.Wait()
}

// dispatcher는 감시 대상의 실행을 대기열에 받아 넣은 순서대로 hook에 넘긴다. 감시를 시작하고 끝낼 때의 hook과
// 대기열 경고 hook도 실행한다.
type dispatcher struct {
	hooks hookFunc
	queue *runQueue
	// warner는 대기열의 단계 변화로 onQueueWarning hook을 실행한다. OnQueueWarning이 없으면 nil이다.
	warner *queueWarner
	// stop은 감시를 멈추면 닫힌다. 닫힌 뒤에는 실행을 받지 않고 hook에 넘기지도 않는다.
	stop <-chan struct{}
	// done은 start가 띄운 고루틴이 끝나거나 skip을 부르면 닫힌다.
	done chan struct{}
}

func newDispatcher(hooks hookFunc, queueSize int, warner *queueWarner, stop <-chan struct{}) *dispatcher {
	d := &dispatcher{hooks: hooks, warner: warner, stop: stop, done: make(chan struct{})}
	d.queue = newRunQueue(queueSize, d.warn)
	return d
}

// start는 onStartWatch를 시작한 뒤 대기열의 실행을 hook에 넘기는 고루틴을 띄운다.
func (d *dispatcher) start() {
	go func() {
		defer close(d.done)
		d.hooks.runStartHook()
		d.run()
	}()
}

// skip은 감시를 시작하지 못해 start를 부르지 않을 때 done을 닫아, finish가 기다리지 않게 한다.
func (d *dispatcher) skip() {
	close(d.done)
}

// enqueue는 실행 runs를 차례로 대기열에 넣는다. 대기열이 가득 차면 기다리지 않고 오류를 돌려준다.
func (d *dispatcher) enqueue(runs []domain.Event) error {
	for _, run := range runs {
		select {
		case <-d.stop:
			return errStopped
		default:
		}
		if err := d.queue.push(run); err != nil {
			return err
		}
	}
	return nil
}

// run은 대기열의 실행을 넣은 순서대로 hook에 넘긴다. 실행은 hook이 시작할 때까지 대기열에 남겨, 자리를 기다리는
// 실행도 대기열의 용량과 경고에 들어가게 한다.
func (d *dispatcher) run() {
	for {
		event, ok := d.queue.first(d.stop)
		if !ok {
			return
		}
		// 감시를 멈춘 뒤에 본 실행은 hook에 넘기지 않는다.
		select {
		case <-d.stop:
			return
		default:
		}
		d.hooks.runChangeHook(event)
		d.queue.pop()
	}
}

// warn은 대기열의 단계 변화 w로 onQueueWarning hook을 실행한다.
func (d *dispatcher) warn(w queueWarning) {
	if d.warner != nil {
		d.warner.notify(w)
	}
}

// cancel은 돌고 있는 hook 실행을 모두 취소하고, 새 실행을 시작하지 않게 한다.
func (d *dispatcher) cancel() {
	d.hooks.cancel()
}

// finish는 done이 닫히기를 기다린 뒤 대기열 경고 hook을 멈추고, started이면 onFinishWatch를 실행한다. 감시를
// 시작하지 못한 대상은 onStartWatch를 실행하지 않았으므로 그 짝인 onFinishWatch도 실행하지 않는다.
func (d *dispatcher) finish(started bool) {
	<-d.done
	if d.warner != nil {
		d.warner.stop()
	}
	if started {
		d.hooks.runFinishHook()
	}
}
