package yaml

import (
	"errors"
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
	Exclude                    config.Exclude     `yaml:"exclude"`
	WaitMillisecond            config.Millisecond `yaml:"waitMillisecond"`
	WatchSubDir                *bool              `yaml:"watchSubDir"`
	WatchFileEvent             *config.WatchEvent `yaml:"watchFileEvent"`
	WatchDirEvent              *config.WatchEvent `yaml:"watchDirEvent"`
	DirEventWaitMillisecond    config.Millisecond `yaml:"dirEventWaitMillisecond"`
	DirEventMaxWaitMillisecond config.Millisecond `yaml:"dirEventMaxWaitMillisecond"`
	NoWait                     *bool              `yaml:"noWait"`
	Interruptible              *bool              `yaml:"interruptible"`

	// RemovedExcludeDir, RemovedExcludeSuffix, RemovedWatchEvent는 v3에서 없앤 키를 받는다. yaml.Unmarshal은
	// 모르는 키를 무시하므로, 키를 받아 두지 않으면 옛 설정에서 뺐던 폴더, 파일, 이벤트가 경고 없이 다시 감시된다.
	RemovedExcludeDir    yaml.Node `yaml:"excludeDir" json:"-"`
	RemovedExcludeSuffix yaml.Node `yaml:"excludeSuffix" json:"-"`
	RemovedWatchEvent    yaml.Node `yaml:"watchEvent" json:"-"`
}

func (c WatchTargetsOptionConfig) BuildOption() (domain.WatchTargetsOption, error) {
	for _, removed := range []struct {
		key, replacement, section string
		node                      yaml.Node
	}{
		{"excludeDir", "option.exclude", "Moving from excludeDir and excludeSuffix", c.RemovedExcludeDir},
		{"excludeSuffix", "option.exclude", "Moving from excludeDir and excludeSuffix", c.RemovedExcludeSuffix},
		{"watchEvent", "option.watchFileEvent and option.watchDirEvent", "Moving from watchEvent", c.RemovedWatchEvent},
	} {
		if removed.node.Kind != 0 {
			return domain.WatchTargetsOption{}, fmt.Errorf(`option.%s is no longer supported; use %s (see USAGE %q)`, removed.key, removed.replacement, removed.section)
		}
	}
	exclude, err := converter.NewExcludeConverter(c.Exclude).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.%w", err)
	}
	watchFileEvent, err := converter.NewWatchFileEventConverter(c.WatchFileEvent).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.%w", err)
	}
	watchDirEvent, err := converter.NewWatchDirEventConverter(c.WatchDirEvent).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.%w", err)
	}
	if watchFileEvent.IsNone() && watchDirEvent.IsNone() {
		return domain.WatchTargetsOption{}, errors.New("option: watchFileEvent and watchDirEvent are both none; onChange would never run")
	}
	return domain.WatchTargetsOption{
		Exclude:         exclude,
		WaitMillisecond: converter.NewWaitMillisecondConverter(c.WaitMillisecond).Convert(),
		WatchSubDir:     converter.NewWatchSubDirConverter(c.WatchSubDir).Convert(),
		WatchFileEvent:  watchFileEvent,
		WatchDirEvent:   watchDirEvent,
		DirEventWait:    domain.Millisecond(c.DirEventWaitMillisecond),
		DirEventMaxWait: domain.Millisecond(c.DirEventMaxWaitMillisecond),
		NoWait:          converter.NewNoWaitConverter(c.NoWait).Convert(),
		Interruptible:   converter.NewInterruptibleConverter(c.Interruptible).Convert(),
	}, nil
}
