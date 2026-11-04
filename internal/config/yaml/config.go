package yaml

import (
	"fmt"

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
	watchTargets, err := c.WatchTargets.BuildCommandSets(shell)
	if err != nil {
		return domain.CommandSet{}, err
	}
	global, err := c.Global.BuildCommandSet(shell)
	if err != nil {
		return domain.CommandSet{}, fmt.Errorf("global.%w", err)
	}
	return domain.CommandSet{
		Global:       global,
		WatchTargets: watchTargets,
	}, nil
}

type GlobalConfig struct {
	LifeCycle GlobalLifeCycleConfig `yaml:"lifeCycle"`
}

func (c GlobalConfig) BuildCommandSet(shell cmd.Shell) (domain.GlobalCommandSet, error) {
	lifeCycle, err := c.LifeCycle.BuildLifeCycle(shell)
	if err != nil {
		return domain.GlobalCommandSet{}, fmt.Errorf("lifeCycle.%w", err)
	}
	return domain.GlobalCommandSet{LifeCycle: lifeCycle}, nil
}

type GlobalLifeCycleConfig struct {
	OnStartWatch   config.CmdInfo `yaml:"onStartWatch"`
	OnBeforeChange config.CmdInfo `yaml:"onBeforeChange"`
	OnAfterChange  config.CmdInfo `yaml:"onAfterChange"`
	OnFinishWatch  config.CmdInfo `yaml:"onFinishWatch"`
	OnError        config.CmdInfo `yaml:"onError"`
}

func (c GlobalLifeCycleConfig) BuildLifeCycle(shell cmd.Shell) (domain.GlobalLifeCycle, error) {
	onBeforeChange, err := converter.NewCmdInfoConverter(c.OnBeforeChange, shell).ConvertEventHook()
	if err != nil {
		return domain.GlobalLifeCycle{}, fmt.Errorf("onBeforeChange%w", err)
	}
	onAfterChange, err := converter.NewCmdInfoConverter(c.OnAfterChange, shell).ConvertEventHook()
	if err != nil {
		return domain.GlobalLifeCycle{}, fmt.Errorf("onAfterChange%w", err)
	}
	return domain.GlobalLifeCycle{
		OnStartWatch:   converter.NewCmdInfoConverter(c.OnStartWatch, shell).Convert(),
		OnBeforeChange: onBeforeChange,
		OnAfterChange:  onAfterChange,
		OnFinishWatch:  converter.NewCmdInfoConverter(c.OnFinishWatch, shell).Convert(),
		OnError:        converter.NewCmdInfoConverter(c.OnError, shell).Convert(),
	}, nil
}

type WatchTargetsConfigs []WatchTargetsConfig

func (cs WatchTargetsConfigs) BuildCommandSets(shell cmd.Shell) (domain.WatchTargetsCommandSets, error) {
	result := domain.WatchTargetsCommandSets{}
	for i, c := range cs {
		commandSet, err := c.BuildCommandSet(shell)
		if err != nil {
			return nil, fmt.Errorf("watchTargets[%d].%w", i, err)
		}
		result = append(result, commandSet)
	}
	return result, nil
}

type WatchTargetsConfig struct {
	Path      config.Path                 `yaml:"path"`
	LifeCycle WatchTargetsLifeCycleConfig `yaml:"lifeCycle"`
	Option    WatchTargetsOptionConfig    `yaml:"option"`
}

func (c WatchTargetsConfig) BuildCommandSet(shell cmd.Shell) (domain.WatchTargetsCommandSet, error) {
	option, err := c.Option.BuildOption()
	if err != nil {
		return domain.WatchTargetsCommandSet{}, err
	}
	lifeCycle, err := c.LifeCycle.BuildLifeCycle(shell)
	if err != nil {
		return domain.WatchTargetsCommandSet{}, fmt.Errorf("lifeCycle.%w", err)
	}
	return domain.WatchTargetsCommandSet{
		Path:      converter.NewPathConverter(c.Path).Convert(),
		LifeCycle: lifeCycle,
		Option:    option,
	}, nil
}

type WatchTargetsLifeCycleConfig struct {
	OnStartWatch  config.CmdInfo `yaml:"onStartWatch"`
	OnChange      config.CmdInfo `yaml:"onChange"`
	OnFinishWatch config.CmdInfo `yaml:"onFinishWatch"`
}

func (c WatchTargetsLifeCycleConfig) BuildLifeCycle(shell cmd.Shell) (domain.WatchTargetsLifeCycle, error) {
	onChange, err := converter.NewCmdInfoConverter(c.OnChange, shell).ConvertEventHook()
	if err != nil {
		return domain.WatchTargetsLifeCycle{}, fmt.Errorf("onChange%w", err)
	}
	return domain.WatchTargetsLifeCycle{
		OnStartWatch:  converter.NewCmdInfoConverter(c.OnStartWatch, shell).Convert(),
		OnChange:      onChange,
		OnFinishWatch: converter.NewCmdInfoConverter(c.OnFinishWatch, shell).Convert(),
	}, nil
}

type WatchTargetsOptionConfig struct {
	Exclude         config.Exclude     `yaml:"exclude"`
	WaitMillisecond config.Millisecond `yaml:"waitMillisecond"`
	WatchSubDir     *bool              `yaml:"watchSubDir"`
	WatchEvent      config.WatchEvent  `yaml:"watchEvent"`
	NoWait          *bool              `yaml:"noWait"`
	Interruptible   *bool              `yaml:"interruptible"`

	// RemovedExcludeDir와 RemovedExcludeSuffix는 v3에서 없앤 키를 받는다. yaml.Unmarshal은 모르는 키를
	// 무시하므로, 키를 받아 두지 않으면 옛 설정에서 뺐던 폴더와 파일이 경고 없이 다시 감시된다.
	RemovedExcludeDir    yaml.Node `yaml:"excludeDir" json:"-"`
	RemovedExcludeSuffix yaml.Node `yaml:"excludeSuffix" json:"-"`
}

func (c WatchTargetsOptionConfig) BuildOption() (domain.WatchTargetsOption, error) {
	for _, removed := range []struct {
		key  string
		node yaml.Node
	}{{"excludeDir", c.RemovedExcludeDir}, {"excludeSuffix", c.RemovedExcludeSuffix}} {
		if removed.node.Kind != 0 {
			return domain.WatchTargetsOption{}, fmt.Errorf(`option.%s is no longer supported; use option.exclude (see USAGE "Moving from excludeDir and excludeSuffix")`, removed.key)
		}
	}
	exclude, err := converter.NewExcludeConverter(c.Exclude).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.%w", err)
	}
	return domain.WatchTargetsOption{
		Exclude:         exclude,
		WaitMillisecond: converter.NewWaitMillisecondConverter(c.WaitMillisecond).Convert(),
		WatchSubDir:     converter.NewWatchSubDirConverter(c.WatchSubDir).Convert(),
		WatchEvent:      converter.NewWatchEventConverter(c.WatchEvent).Convert(),
		NoWait:          converter.NewNoWaitConverter(c.NoWait).Convert(),
		Interruptible:   converter.NewInterruptibleConverter(c.Interruptible).Convert(),
	}, nil
}
