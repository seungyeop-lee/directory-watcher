package yaml

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
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
		return domain.CommandSet{}, err
	}
	return domain.CommandSet{
		Global:       global,
		WatchTargets: watchTargets,
	}, nil
}

type GlobalConfig struct {
	LifeCycle GlobalLifeCycleConfig `yaml:"lifeCycle"`
}

func (c GlobalConfig) BuildCommandSet(shell command.Shell) (domain.GlobalCommandSet, error) {
	lifeCycle, err := c.LifeCycle.BuildLifeCycle(shell)
	if err != nil {
		return domain.GlobalCommandSet{}, converter.WithKey("global", err)
	}
	return domain.GlobalCommandSet{LifeCycle: lifeCycle}, nil
}

type GlobalLifeCycleConfig struct {
	OnStartWatch   config.CmdInfo `yaml:"onStartWatch"`
	OnBeforeChange config.CmdInfo `yaml:"onBeforeChange"`
	OnAfterChange  config.CmdInfo `yaml:"onAfterChange"`
	OnFinishWatch  config.CmdInfo `yaml:"onFinishWatch"`
	OnError        config.CmdInfo `yaml:"onError"`
	OnQueueWarning config.CmdInfo `yaml:"onQueueWarning"`
}

func (c GlobalLifeCycleConfig) BuildLifeCycle(shell command.Shell) (domain.GlobalLifeCycle, error) {
	var lifeCycle domain.GlobalLifeCycle
	err := buildHooks([]hook{
		{converter.NewCmdInfoConverter("onStartWatch", c.OnStartWatch, shell), &lifeCycle.OnStartWatch},
		{converter.NewEventHookConverter("onBeforeChange", c.OnBeforeChange, shell), &lifeCycle.OnBeforeChange},
		{converter.NewEventHookConverter("onAfterChange", c.OnAfterChange, shell), &lifeCycle.OnAfterChange},
		{converter.NewCmdInfoConverter("onFinishWatch", c.OnFinishWatch, shell), &lifeCycle.OnFinishWatch},
		{converter.NewCmdInfoConverter("onError", c.OnError, shell), &lifeCycle.OnError},
		{converter.NewCmdInfoConverter("onQueueWarning", c.OnQueueWarning, shell), &lifeCycle.OnQueueWarning},
	})
	if err != nil {
		return domain.GlobalLifeCycle{}, err
	}
	return lifeCycle, nil
}

type WatchTargetsConfigs []WatchTargetsConfig

func (cs WatchTargetsConfigs) BuildCommandSets(shell command.Shell) (domain.WatchTargetsCommandSets, error) {
	result := domain.WatchTargetsCommandSets{}
	for i, c := range cs {
		commandSet, err := c.BuildCommandSet(shell)
		if err != nil {
			return nil, converter.WithKey(fmt.Sprintf("watchTargets[%d]", i), err)
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

func (c WatchTargetsConfig) BuildCommandSet(shell command.Shell) (domain.WatchTargetsCommandSet, error) {
	option, err := c.Option.BuildOption()
	if err != nil {
		return domain.WatchTargetsCommandSet{}, err
	}
	lifeCycle, err := c.LifeCycle.BuildLifeCycle(shell)
	if err != nil {
		return domain.WatchTargetsCommandSet{}, err
	}
	path, err := converter.NewPathConverter(c.Path).Convert()
	if err != nil {
		return domain.WatchTargetsCommandSet{}, err
	}
	return domain.WatchTargetsCommandSet{
		Path:      path,
		LifeCycle: lifeCycle,
		Option:    option,
	}, nil
}

type WatchTargetsLifeCycleConfig struct {
	OnStartWatch  config.CmdInfo `yaml:"onStartWatch"`
	OnChange      config.CmdInfo `yaml:"onChange"`
	OnFinishWatch config.CmdInfo `yaml:"onFinishWatch"`
}

func (c WatchTargetsLifeCycleConfig) BuildLifeCycle(shell command.Shell) (domain.WatchTargetsLifeCycle, error) {
	var lifeCycle domain.WatchTargetsLifeCycle
	err := buildHooks([]hook{
		{converter.NewCmdInfoConverter("onStartWatch", c.OnStartWatch, shell), &lifeCycle.OnStartWatch},
		{converter.NewEventHookConverter("onChange", c.OnChange, shell), &lifeCycle.OnChange},
		{converter.NewCmdInfoConverter("onFinishWatch", c.OnFinishWatch, shell), &lifeCycle.OnFinishWatch},
	})
	if err != nil {
		return domain.WatchTargetsLifeCycle{}, err
	}
	return lifeCycle, nil
}

// hook은 lifeCycle 아래 hook 하나를 바꾸는 converter와, 바꾼 명령을 담을 곳이다.
type hook struct {
	converter *converter.CmdInfoConverter
	cmd       *domain.Cmd
}

// buildHooks는 hooks를 차례대로 바꿔 각 cmd에 담는다. 오류는 lifeCycle로 시작한다.
func buildHooks(hooks []hook) error {
	for _, h := range hooks {
		built, err := h.converter.Convert()
		if err != nil {
			return converter.WithKey("lifeCycle", err)
		}
		*h.cmd = built
	}
	return nil
}
