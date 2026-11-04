package domain

type CommandSet struct {
	Global       GlobalCommandSet
	WatchTargets WatchTargetsCommandSets
}

type GlobalCommandSet struct {
	LifeCycle GlobalLifeCycle
}

type GlobalLifeCycle struct {
	OnStartWatch   Cmd
	OnBeforeChange Cmd
	OnAfterChange  Cmd
	OnFinishWatch  Cmd
	OnError        Cmd
	OnQueueWarning Cmd
}

type WatchTargetsCommandSets []WatchTargetsCommandSet

type WatchTargetsCommandSet struct {
	Path      Path
	LifeCycle WatchTargetsLifeCycle
	Option    WatchTargetsOption
}

type WatchTargetsLifeCycle struct {
	OnStartWatch  Cmd
	OnChange      Cmd
	OnFinishWatch Cmd
}

// WatchTargetsOption은 감시 대상 옵션을 설정 파일의 option 아래 그룹과 같은 이름으로 나눠 담는다.
type WatchTargetsOption struct {
	Watch     WatchOption
	DirBatch  DirBatchOption
	HookBatch HookBatchOption
	Queue     QueueOption
	Run       RunPolicy
}

type WatchOption struct {
	SubDir    bool
	Exclude   ExcludePatterns
	FileEvent WatchEvent
	DirEvent  WatchEvent
}

// DirBatchOption은 폴더 이벤트를 만들려고 이벤트를 모으는 시간이다. WaitMillisecond가 0이면 모으지 않는다.
type DirBatchOption struct {
	WaitMillisecond    Millisecond
	MaxWaitMillisecond Millisecond
}

// HookBatchOption은 hook을 실행하기 전에 이벤트를 모으는 시간과 모은 이벤트를 hook에 넘기는 방식이다.
// WaitMillisecond가 0이면 모으지 않는다.
type HookBatchOption struct {
	WaitMillisecond    Millisecond
	MaxWaitMillisecond Millisecond
	Deliver            Deliver
}

// QueueOption의 Size는 감시 대상의 대기열에 둘 수 있는 실행의 수다. 1 이상이다.
type QueueOption struct {
	Size int
}
