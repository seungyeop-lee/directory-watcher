package yaml

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/config"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/config/converter"
	"gopkg.in/yaml.v3"
)

var _ config.Config = (*Config)(nil)

func NewConfig(configFile []byte) config.Config {
	c := Config{}
	if err := yaml.Unmarshal(configFile, &c); err != nil {
		panic(err)
	}
	return &c
}

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
	OnStartWatch   converter.CmdInfo `yaml:"onStartWatch"`
	OnBeforeChange converter.CmdInfo `yaml:"onBeforeChange"`
	OnAfterChange  converter.CmdInfo `yaml:"onAfterChange"`
	OnFinishWatch  converter.CmdInfo `yaml:"onFinishWatch"`
}

func (c GlobalLifeCycleConfig) BuildLifeCycle() domain.GlobalLifeCycle {
	return domain.GlobalLifeCycle{
		OnStartWatch:   converter.NewCmdInfoConverter(c.OnStartWatch).Convert(),
		OnBeforeChange: converter.NewCmdInfoConverter(c.OnBeforeChange).Convert(),
		OnAfterChange:  converter.NewCmdInfoConverter(c.OnAfterChange).Convert(),
		OnFinishWatch:  converter.NewCmdInfoConverter(c.OnFinishWatch).Convert(),
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
	Path      converter.Path              `yaml:"path"`
	LifeCycle WatchTargetsLifeCycleConfig `yaml:"lifeCycle"`
	Option    WatchTargetsOptionConfig    `yaml:"option"`
}

func (c WatchTargetsConfig) BuildCommandSet() domain.WatchTargetsCommandSet {
	return domain.WatchTargetsCommandSet{
		Path:      converter.NewPathConverter(c.Path).Convert(),
		LifeCycle: c.LifeCycle.BuildLifeCycle(),
		Option:    c.Option.BuildOption(),
	}
}

type WatchTargetsLifeCycleConfig struct {
	OnStartWatch  converter.CmdInfo `yaml:"onStartWatch"`
	OnChange      converter.CmdInfo `yaml:"onChange"`
	OnFinishWatch converter.CmdInfo `yaml:"onFinishWatch"`
}

func (c WatchTargetsLifeCycleConfig) BuildLifeCycle() domain.WatchTargetsLifeCycle {
	return domain.WatchTargetsLifeCycle{
		OnStartWatch:  converter.NewCmdInfoConverter(c.OnStartWatch).Convert(),
		OnChange:      converter.NewCmdInfoConverter(c.OnChange).Convert(),
		OnFinishWatch: converter.NewCmdInfoConverter(c.OnFinishWatch).Convert(),
	}
}

type WatchTargetsOptionConfig struct {
	ExcludeDir      converter.Paths        `yaml:"excludeDir"`
	ExcludeSuffix   converter.PathSuffixes `yaml:"excludeSuffix"`
	WaitMillisecond converter.Millisecond  `yaml:"waitMillisecond"`
	WatchSubDir     *bool                  `yaml:"watchSubDir"`
}

func (c WatchTargetsOptionConfig) BuildOption() domain.WatchTargetsOption {
	return domain.WatchTargetsOption{
		ExcludeDir:      converter.NewPathsConverter(c.ExcludeDir).Convert(),
		ExcludeSuffix:   converter.NewPathSuffixesConverter(c.ExcludeSuffix).Convert(),
		WaitMillisecond: converter.NewWaitMillisecondConverter(c.WaitMillisecond).Convert(),
		WatchSubDir:     converter.NewWatchSubDirConverter(c.WatchSubDir).Convert(),
	}
}
