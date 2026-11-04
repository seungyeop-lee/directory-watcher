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

type WatchTargetsOption struct {
	Exclude ExcludePatterns
	// WaitMillisecond와 MaxWaitMillisecond는 hook을 실행하기 전에 이벤트를 모으는 시간이다. WaitMillisecond가
	// 0이면 모으지 않는다.
	WaitMillisecond    Millisecond
	MaxWaitMillisecond Millisecond
	Deliver            Deliver
	WatchSubDir        bool
	WatchFileEvent     WatchEvent
	WatchDirEvent      WatchEvent
	// DirEventWait와 DirEventMaxWait는 폴더 이벤트를 모으는 시간이다. DirEventWait가 0이면 모으지 않는다.
	DirEventWait    Millisecond
	DirEventMaxWait Millisecond
	// QueueSize는 감시 대상의 대기열에 둘 수 있는 실행의 수다. 1 이상이다.
	QueueSize int
	Run       RunPolicy
}
