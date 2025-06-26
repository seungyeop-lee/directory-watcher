package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
	"github.com/stretchr/testify/assert"
)

// Mock Logger for testing
type mockLogger struct{}

func (m *mockLogger) Info(msg string)  {}
func (m *mockLogger) Error(msg string) {}
func (m *mockLogger) Debug(msg string) {}

func newMockLogger() helper.Logger {
	return &mockLogger{}
}

// Simplified NewEvent for testing
func newTestEvent(path domain.Path, op fsnotify.Op) domain.Event {
	return domain.Event{
		Path:      path,
		Operation: domain.Operation(op),
	}
}

func TestWatchTargetRunner_Debounce_Bug(t *testing.T) {
	// Given
	var changeHookCallCount int32
	var wg sync.WaitGroup
	wg.Add(1)

	// Mock runChangeHook
	mockRunChangeHook := func(event domain.Event) {
		atomic.AddInt32(&changeHookCallCount, 1)
		wg.Done()
	}

	runner := &watchTargetRunner{
		runChangeHook:   mockRunChangeHook,
		excludeSuffix:   domain.PathSuffixes{".swp"},
		waitMillisecond: 100, // 100ms 대기
		noWait:          false,
		logger:          newMockLogger(),
	}

	eventChan := make(chan domain.Event)
	go runner.selectEventHandler()(eventChan)

	// When
	// 1. 유효한 이벤트를 보냄
	eventChan <- newTestEvent(domain.Path("/test/test.go"), fsnotify.Write)

	// 2. 짧은 시간 후 무시할 이벤트를 보냄
	time.Sleep(10 * time.Millisecond)
	eventChan <- newTestEvent(domain.Path("/test/test.swp"), fsnotify.Write)

	// 3. 대기 후에도 추가적인 호출이 없는지 확인
	// wg.Wait()를 여기서 하면 안됨. 두번째 이벤트에 의해 첫번째 이벤트가 다시 실행되는 버그 상황에서는
	// wg.Done()이 두번 호출되어 panic이 발생함.

	time.Sleep(200 * time.Millisecond) // 디바운스 시간보다 길게 대기하여 모든 호출이 발생하도록 함

	// Then
	assert.Equal(t, int32(1), atomic.LoadInt32(&changeHookCallCount), "runChangeHook should be called only once")
}
