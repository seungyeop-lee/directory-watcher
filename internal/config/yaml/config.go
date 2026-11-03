package yaml

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/cmd"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config/converter"
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
	Shell        config.Shell        `yaml:"shell"`
	Global       GlobalConfig        `yaml:"global"`
	WatchTargets WatchTargetsConfigs `yaml:"watchTargets"`
}

func (c Config) BuildCommandSet() (domain.CommandSet, error) {
	shell, err := converter.NewShellConverter(c.Shell).Convert()
	if err != nil {
		return domain.CommandSet{}, err
	}
	return domain.CommandSet{
		Global:       c.Global.BuildCommandSet(shell),
		WatchTargets: c.WatchTargets.BuildCommandSets(shell),
	}, nil
}

type GlobalConfig struct {
	LifeCycle GlobalLifeCycleConfig `yaml:"lifeCycle"`
}

func (c GlobalConfig) BuildCommandSet(shell cmd.Shell) domain.GlobalCommandSet {
	return domain.GlobalCommandSet{
		LifeCycle: c.LifeCycle.BuildLifeCycle(shell),
	}
}

type GlobalLifeCycleConfig struct {
	OnStartWatch   config.CmdInfo `yaml:"onStartWatch"`
	OnBeforeChange config.CmdInfo `yaml:"onBeforeChange"`
	OnAfterChange  config.CmdInfo `yaml:"onAfterChange"`
	OnFinishWatch  config.CmdInfo `yaml:"onFinishWatch"`
	OnError        config.CmdInfo `yaml:"onError"`
}

func (c GlobalLifeCycleConfig) BuildLifeCycle(shell cmd.Shell) domain.GlobalLifeCycle {
	return domain.GlobalLifeCycle{
		OnStartWatch:   converter.NewCmdInfoConverter(c.OnStartWatch, shell).Convert(),
		OnBeforeChange: converter.NewCmdInfoConverter(c.OnBeforeChange, shell).Convert(),
		OnAfterChange:  converter.NewCmdInfoConverter(c.OnAfterChange, shell).Convert(),
		OnFinishWatch:  converter.NewCmdInfoConverter(c.OnFinishWatch, shell).Convert(),
		OnError:        converter.NewCmdInfoConverter(c.OnError, shell).Convert(),
	}
}

type WatchTargetsConfigs []WatchTargetsConfig

func (cs WatchTargetsConfigs) BuildCommandSets(shell cmd.Shell) domain.WatchTargetsCommandSets {
	result := domain.WatchTargetsCommandSets{}
	for _, c := range cs {
		result = append(result, c.BuildCommandSet(shell))
	}
	return result
}

type WatchTargetsConfig struct {
	Path      config.Path                 `yaml:"path"`
	LifeCycle WatchTargetsLifeCycleConfig `yaml:"lifeCycle"`
	Option    WatchTargetsOptionConfig    `yaml:"option"`
}

func (c WatchTargetsConfig) BuildCommandSet(shell cmd.Shell) domain.WatchTargetsCommandSet {
	return domain.WatchTargetsCommandSet{
		Path:      converter.NewPathConverter(c.Path).Convert(),
		LifeCycle: c.LifeCycle.BuildLifeCycle(shell),
		Option:    c.Option.BuildOption(),
	}
}

type WatchTargetsLifeCycleConfig struct {
	OnStartWatch  config.CmdInfo `yaml:"onStartWatch"`
	OnChange      config.CmdInfo `yaml:"onChange"`
	OnFinishWatch config.CmdInfo `yaml:"onFinishWatch"`
}

func (c WatchTargetsLifeCycleConfig) BuildLifeCycle(shell cmd.Shell) domain.WatchTargetsLifeCycle {
	return domain.WatchTargetsLifeCycle{
		OnStartWatch:  converter.NewCmdInfoConverter(c.OnStartWatch, shell).Convert(),
		OnChange:      converter.NewCmdInfoConverter(c.OnChange, shell).Convert(),
		OnFinishWatch: converter.NewCmdInfoConverter(c.OnFinishWatch, shell).Convert(),
	}
}

type WatchTargetsOptionConfig struct {
	ExcludeDir      config.Paths        `yaml:"excludeDir"`
	ExcludeSuffix   config.PathSuffixes `yaml:"excludeSuffix"`
	WaitMillisecond config.Millisecond  `yaml:"waitMillisecond"`
	WatchSubDir     *bool               `yaml:"watchSubDir"`
	WatchEvent      config.WatchEvent   `yaml:"watchEvent"`
	NoWait          *bool               `yaml:"noWait"`
	Interruptible   *bool               `yaml:"interruptible"`
}

func (c WatchTargetsOptionConfig) BuildOption() domain.WatchTargetsOption {
	return domain.WatchTargetsOption{
		ExcludeDir:      converter.NewPathsConverter(c.ExcludeDir).Convert(),
		ExcludeSuffix:   converter.NewPathSuffixesConverter(c.ExcludeSuffix).Convert(),
		WaitMillisecond: converter.NewWaitMillisecondConverter(c.WaitMillisecond).Convert(),
		WatchSubDir:     converter.NewWatchSubDirConverter(c.WatchSubDir).Convert(),
		WatchEvent:      converter.NewWatchEventConverter(c.WatchEvent).Convert(),
		NoWait:          converter.NewNoWaitConverter(c.NoWait).Convert(),
		Interruptible:   converter.NewInterruptibleConverter(c.Interruptible).Convert(),
	}
}
