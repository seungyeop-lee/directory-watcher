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
	Exclude         ExcludePatterns
	WaitMillisecond Millisecond
	WatchSubDir     bool
	WatchFileEvent  WatchEvent
	WatchDirEvent   WatchEvent
	// DirEventWait와 DirEventMaxWait는 폴더 이벤트를 모으는 시간이다. 0이면 기본값을 쓴다.
	DirEventWait    Millisecond
	DirEventMaxWait Millisecond
	NoWait          bool
	Interruptible   Interruptible
}
