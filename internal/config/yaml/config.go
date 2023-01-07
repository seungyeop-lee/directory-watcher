package yaml

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type CmdInfo interface{}
type Path string
type Paths []Path
type PathSuffixes []string
type Millisecond uint

type Config struct {
	Global       GlobalConfig        `yaml:"global"`
	WatchTargets WatchTargetsConfigs `yaml:"watchTargets"`
}

func (c Config) BuildCommandSet() domain.CommandSet {
	return domain.CommandSet{
		Global:       c.Global.BuildCommandSet(),
		WatchTargets: c.WatchTargets.BuildCommandSets(),
	}
}

type GlobalConfig struct {
	LifeCycle GlobalLifeCycleConfig `yaml:"lifeCycle"`
}

func (c GlobalConfig) BuildCommandSet() domain.GlobalCommandSet {
	return domain.GlobalCommandSet{
		LifeCycle: c.LifeCycle.BuildLifeCycle(),
	}
}

type GlobalLifeCycleConfig struct {
	OnStartWatch   CmdInfo `yaml:"onStartWatch"`
	OnBeforeChange CmdInfo `yaml:"onBeforeChange"`
	OnAfterChange  CmdInfo `yaml:"onAfterChange"`
	OnFinishWatch  CmdInfo `yaml:"onFinishWatch"`
}

func (c GlobalLifeCycleConfig) BuildLifeCycle() domain.GlobalLifeCycle {
	return domain.GlobalLifeCycle{
		OnStartWatch:   NewCmdInfoConverter(c.OnStartWatch).Convert(),
		OnBeforeChange: NewCmdInfoConverter(c.OnBeforeChange).Convert(),
		OnAfterChange:  NewCmdInfoConverter(c.OnAfterChange).Convert(),
		OnFinishWatch:  NewCmdInfoConverter(c.OnFinishWatch).Convert(),
	}
}

type WatchTargetsConfigs []WatchTargetsConfig

func (cs WatchTargetsConfigs) BuildCommandSets() domain.WatchTargetsCommandSets {
	result := domain.WatchTargetsCommandSets{}
	for _, c := range cs {
		result = append(result, c.BuildCommandSet())
	}
	return result
}

type WatchTargetsConfig struct {
	Path      Path                        `yaml:"path"`
	LifeCycle WatchTargetsLifeCycleConfig `yaml:"lifeCycle"`
	Option    WatchTargetsOptionConfig    `yaml:"option"`
}

func (c WatchTargetsConfig) BuildCommandSet() domain.WatchTargetsCommandSet {
	return domain.WatchTargetsCommandSet{
		Path:      NewPathConverter(c.Path).Convert(),
		LifeCycle: c.LifeCycle.BuildLifeCycle(),
		Option:    c.Option.BuildOption(),
	}
}

type WatchTargetsLifeCycleConfig struct {
	OnStartWatch  CmdInfo `yaml:"onStartWatch"`
	OnChange      CmdInfo `yaml:"onChange"`
	OnFinishWatch CmdInfo `yaml:"onFinishWatch"`
}

func (c WatchTargetsLifeCycleConfig) BuildLifeCycle() domain.WatchTargetsLifeCycle {
	return domain.WatchTargetsLifeCycle{
		OnStartWatch:  NewCmdInfoConverter(c.OnStartWatch).Convert(),
		OnChange:      NewCmdInfoConverter(c.OnChange).Convert(),
		OnFinishWatch: NewCmdInfoConverter(c.OnFinishWatch).Convert(),
	}
}

type WatchTargetsOptionConfig struct {
	ExcludeDir      Paths        `yaml:"excludeDir"`
	ExcludeSuffix   PathSuffixes `yaml:"excludeSuffix"`
	WaitMillisecond Millisecond  `yaml:"waitMillisecond"`
}

func (c WatchTargetsOptionConfig) BuildOption() domain.WatchTargetsOption {
	return domain.WatchTargetsOption{
		ExcludeDir:      NewPathsConverter(c.ExcludeDir).Convert(),
		ExcludeSuffix:   NewPathSuffixesConverter(c.ExcludeSuffix).Convert(),
		WaitMillisecond: NewWaitMillisecondConverter(c.WaitMillisecond).Convert(),
	}
}
