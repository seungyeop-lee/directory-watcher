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
	WatchEvent      WatchEvent
	NoWait          bool
	Interruptible   Interruptible
}
