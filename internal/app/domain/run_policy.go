package domain

// Interrupt는 새 실행을 시작하기 전에 돌고 있는 어떤 실행을 취소할지 정한다.
type Interrupt int

const (
	// InterruptNone은 아무 실행도 취소하지 않고 자리가 날 때까지 기다린다.
	InterruptNone Interrupt = iota
	// InterruptSamePath는 새 실행과 같은 경로를 처리하던 실행을 취소한다. 그런 실행이 없으면 자리가 날 때까지 기다린다.
	InterruptSamePath
	// InterruptOldest는 자리가 다 찼을 때 가장 먼저 시작한 실행을 취소한다.
	InterruptOldest
)

// RunPolicy는 감시 대상의 hook 실행을 동시에 Concurrency개까지 돌리고, 새 실행이 Interrupt에 따라 돌고 있는
// 실행을 취소하게 한다. Concurrency는 1 이상이다.
type RunPolicy struct {
	Concurrency int
	Interrupt   Interrupt
}
