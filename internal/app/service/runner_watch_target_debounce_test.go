package service

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
	"github.com/stretchr/testify/assert"
)

// mockLogger는 테스트용 모의 Logger다.
type mockLogger struct{}

func (m *mockLogger) Info(msg string)  {}
func (m *mockLogger) Error(msg string) {}
func (m *mockLogger) Debug(msg string) {}

func newMockLogger() helper.Logger {
	return &mockLogger{}
}

// newTestEvent는 domain.NewEventByFsnotify와 같은 Event를 만들되, fsnotify.Event 대신 경로와 연산을 따로 받는다.
func newTestEvent(path domain.Path, op fsnotify.Op) domain.Event {
	return domain.Event{
		Path:      path,
		Operation: domain.Operation(op),
	}
}

func TestWatchTargetRunner_Debounce_Bug(t *testing.T) {
	// 준비
	var changeHookCallCount int32

	// 모의 runChangeHook
	mockRunChangeHook := func(event domain.Event) {
		atomic.AddInt32(&changeHookCallCount, 1)
	}

	runner := &watchTargetRunner{
		runChangeHook:   mockRunChangeHook,
		excludeSuffix:   domain.PathSuffixes{".swp"},
		waitMillisecond: 100, // 100ms 대기
		noWait:          false,
		logger:          newMockLogger(),
	}

	eventChan := make(chan domain.Event)
	done := make(chan struct{})
	go func() { runner.selectEventHandler()(eventChan); close(done) }()
	defer func() { close(eventChan); <-done }()

	// 실행
	// 1. 유효한 이벤트를 보냄
	eventChan <- newTestEvent(domain.Path("/test/test.go"), fsnotify.Write)

	// 2. 짧은 시간 후 무시할 이벤트를 보냄
	time.Sleep(10 * time.Millisecond)
	eventChan <- newTestEvent(domain.Path("/test/test.swp"), fsnotify.Write)

	time.Sleep(200 * time.Millisecond) // 디바운스 시간보다 길게 대기하여 모든 호출이 발생하도록 함

	// 검증
	assert.Equal(t, int32(1), atomic.LoadInt32(&changeHookCallCount), "runChangeHook should be called only once")
}
