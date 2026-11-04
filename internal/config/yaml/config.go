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
	OnQueueWarning config.CmdInfo `yaml:"onQueueWarning"`
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
		OnQueueWarning: converter.NewCmdInfoConverter(c.OnQueueWarning, shell).Convert(),
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

// WatchTargetsOptionConfig는 감시 대상 옵션을 처리 단계별 그룹으로 받는다. watch는 무엇을 감시할지,
// dirBatch는 폴더 이벤트를 만들려고 이벤트를 모으는 방식, hookBatch는 hook을 실행하기 전에 이벤트를 모으고 합치는
// 방식, queue는 hook 실행을 기다리는 대기열을 정한다.
type WatchTargetsOptionConfig struct {
	Watch         WatchOptionConfig     `yaml:"watch"`
	DirBatch      DirBatchOptionConfig  `yaml:"dirBatch"`
	HookBatch     HookBatchOptionConfig `yaml:"hookBatch"`
	Queue         QueueOptionConfig     `yaml:"queue"`
	Interruptible *bool                 `yaml:"interruptible"`

	// Removed로 시작하는 필드는 v3에서 없애거나 그룹 안으로 옮긴 키를 받는다. yaml.Unmarshal은 모르는 키를 무시하므로,
	// 키를 받아 두지 않으면 옛 설정에서 뺐던 폴더, 파일, 이벤트가 경고 없이 다시 감시되고, 하위 폴더 감시 여부와 기다리는 시간도 경고 없이 기본값으로 돌아간다.
	RemovedExcludeDir      yaml.Node `yaml:"excludeDir" json:"-"`
	RemovedExcludeSuffix   yaml.Node `yaml:"excludeSuffix" json:"-"`
	RemovedExclude         yaml.Node `yaml:"exclude" json:"-"`
	RemovedWatchEvent      yaml.Node `yaml:"watchEvent" json:"-"`
	RemovedWatchSubDir     yaml.Node `yaml:"watchSubDir" json:"-"`
	RemovedWaitMillisecond yaml.Node `yaml:"waitMillisecond" json:"-"`
	RemovedNoWait          yaml.Node `yaml:"noWait" json:"-"`
}

type WatchOptionConfig struct {
	SubDir    *bool              `yaml:"subDir"`
	Exclude   config.Exclude     `yaml:"exclude"`
	FileEvent *config.WatchEvent `yaml:"fileEvent"`
	DirEvent  *config.WatchEvent `yaml:"dirEvent"`
}

// 기다리는 시간은 적지 않은 것과 0을 구별하려고 포인터로 받는다. waitMillisecond가 0이면 이벤트를 모으지 않는다.
type DirBatchOptionConfig struct {
	WaitMillisecond    *config.Millisecond `yaml:"waitMillisecond"`
	MaxWaitMillisecond *config.Millisecond `yaml:"maxWaitMillisecond"`
}

type HookBatchOptionConfig struct {
	WaitMillisecond    *config.Millisecond `yaml:"waitMillisecond"`
	MaxWaitMillisecond *config.Millisecond `yaml:"maxWaitMillisecond"`
	Deliver            *string             `yaml:"deliver"`
}

// size는 적지 않은 것과 구별하려고 포인터로, 0 이하를 적은 것을 오류로 알리려고 부호 있는 정수로 받는다.
type QueueOptionConfig struct {
	Size *int `yaml:"size"`
}

// 기다리는 시간을 적지 않았을 때 쓰는 값이다.
const defaultWaitMillisecond, defaultMaxWaitMillisecond config.Millisecond = 100, 1000

// defaultQueueSize는 queue.size를 적지 않았을 때 쓰는 대기열 용량이다.
const defaultQueueSize = 2048

func (c WatchTargetsOptionConfig) BuildOption() (domain.WatchTargetsOption, error) {
	const excludeSection, watchEventSection, optionsSection = "Moving from excludeDir and excludeSuffix", "Moving from watchEvent", "Moving from version 2 options"
	for _, removed := range []struct {
		key, replacement, section string
		node                      yaml.Node
	}{
		{"excludeDir", "option.watch.exclude", excludeSection, c.RemovedExcludeDir},
		{"excludeSuffix", "option.watch.exclude", excludeSection, c.RemovedExcludeSuffix},
		{"exclude", "option.watch.exclude", excludeSection, c.RemovedExclude},
		{"watchEvent", "option.watch.fileEvent and option.watch.dirEvent", watchEventSection, c.RemovedWatchEvent},
		{"watchSubDir", "option.watch.subDir", optionsSection, c.RemovedWatchSubDir},
		{"waitMillisecond", "option.hookBatch.waitMillisecond", optionsSection, c.RemovedWaitMillisecond},
		{"noWait", "option.hookBatch.waitMillisecond: 0 and option.hookBatch.deliver: each in place of noWait: true, or remove noWait", optionsSection, c.RemovedNoWait},
	} {
		if removed.node.Kind != 0 {
			return domain.WatchTargetsOption{}, fmt.Errorf(`option.%s is no longer supported; use %s (see USAGE %q)`, removed.key, removed.replacement, removed.section)
		}
	}
	exclude, err := converter.NewExcludeConverter(c.Watch.Exclude).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.watch.%w", err)
	}
	fileEvent, err := converter.NewWatchFileEventConverter(c.Watch.FileEvent).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.watch.%w", err)
	}
	dirEvent, err := converter.NewWatchDirEventConverter(c.Watch.DirEvent).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.watch.%w", err)
	}
	if fileEvent.IsNone() && dirEvent.IsNone() {
		return domain.WatchTargetsOption{}, errors.New("option.watch: fileEvent and dirEvent are both none; onChange would never run")
	}
	wait := converter.NewMillisecondConverter(c.HookBatch.WaitMillisecond, defaultWaitMillisecond).Convert()
	maxWait := converter.NewMillisecondConverter(c.HookBatch.MaxWaitMillisecond, defaultMaxWaitMillisecond).Convert()
	if maxWait < wait {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.hookBatch: maxWaitMillisecond %d is less than waitMillisecond %d", maxWait, wait)
	}
	deliver, err := converter.NewDeliverConverter(c.HookBatch.Deliver).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.hookBatch.%w", err)
	}
	queueSize := defaultQueueSize
	if c.Queue.Size != nil {
		queueSize = *c.Queue.Size
	}
	if queueSize < 1 {
		return domain.WatchTargetsOption{}, fmt.Errorf("option.queue: size %d is less than 1", queueSize)
	}
	return domain.WatchTargetsOption{
		Exclude:            exclude,
		WaitMillisecond:    wait,
		MaxWaitMillisecond: maxWait,
		Deliver:            deliver,
		WatchSubDir:        converter.NewWatchSubDirConverter(c.Watch.SubDir).Convert(),
		WatchFileEvent:     fileEvent,
		WatchDirEvent:      dirEvent,
		DirEventWait:       converter.NewMillisecondConverter(c.DirBatch.WaitMillisecond, defaultWaitMillisecond).Convert(),
		DirEventMaxWait:    converter.NewMillisecondConverter(c.DirBatch.MaxWaitMillisecond, defaultMaxWaitMillisecond).Convert(),
		QueueSize:          queueSize,
		Interruptible:      converter.NewInterruptibleConverter(c.Interruptible).Convert(),
	}, nil
}
