package yaml

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config/converter"
	"gopkg.in/yaml.v3"
)

// WatchTargetsOptionConfig는 감시 대상 옵션을 처리 단계별 그룹으로 받는다. watch는 무엇을 감시할지,
// dirBatch는 폴더 이벤트를 만들려고 이벤트를 모으는 방식, hookBatch는 hook을 실행하기 전에 이벤트를 모으고 합치는
// 방식, queue는 hook 실행을 기다리는 대기열, run은 hook을 동시에 몇 개까지 실행하고 어떤 실행을 취소할지 정한다.
type WatchTargetsOptionConfig struct {
	Watch     WatchOptionConfig     `yaml:"watch"`
	DirBatch  DirBatchOptionConfig  `yaml:"dirBatch"`
	HookBatch HookBatchOptionConfig `yaml:"hookBatch"`
	Queue     QueueOptionConfig     `yaml:"queue"`
	Run       RunOptionConfig       `yaml:"run"`

	// Removed로 시작하는 필드는 v3에서 없애거나 그룹 안으로 옮긴 키를 받는다. yaml.Unmarshal은 모르는 키를 무시하므로,
	// 키를 받아 두지 않으면 옛 설정에서 뺐던 폴더, 파일, 이벤트가 경고 없이 다시 감시되고, 하위 폴더 감시 여부와 기다리는 시간,
	// 실행을 취소하는 방식도 경고 없이 기본값으로 돌아간다.
	RemovedExcludeDir      yaml.Node `yaml:"excludeDir" json:"-"`
	RemovedExcludeSuffix   yaml.Node `yaml:"excludeSuffix" json:"-"`
	RemovedExclude         yaml.Node `yaml:"exclude" json:"-"`
	RemovedWatchEvent      yaml.Node `yaml:"watchEvent" json:"-"`
	RemovedWatchSubDir     yaml.Node `yaml:"watchSubDir" json:"-"`
	RemovedWaitMillisecond yaml.Node `yaml:"waitMillisecond" json:"-"`
	RemovedNoWait          yaml.Node `yaml:"noWait" json:"-"`
	RemovedInterruptible   yaml.Node `yaml:"interruptible" json:"-"`
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

// concurrency는 적지 않은 것과 구별하려고 포인터로, 0 이하를 적은 것을 오류로 알리려고 부호 있는 정수로 받는다.
type RunOptionConfig struct {
	Concurrency *int    `yaml:"concurrency"`
	Interrupt   *string `yaml:"interrupt"`
}

// BuildOption은 없앤 키가 없는지 본 뒤, option 아래 그룹마다 converter로 값을 바꾼다. 오류는 option으로 시작한다.
func (c WatchTargetsOptionConfig) BuildOption() (domain.WatchTargetsOption, error) {
	if err := c.checkRemovedKeys(); err != nil {
		return domain.WatchTargetsOption{}, converter.WithKey("option", err)
	}
	watch, err := converter.NewWatchConverter(c.Watch.SubDir, c.Watch.Exclude, c.Watch.FileEvent, c.Watch.DirEvent).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, converter.WithKey("option", err)
	}
	dirBatch, err := converter.NewDirBatchConverter(c.DirBatch.WaitMillisecond, c.DirBatch.MaxWaitMillisecond).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, converter.WithKey("option", err)
	}
	hookBatch, err := converter.NewHookBatchConverter(c.HookBatch.WaitMillisecond, c.HookBatch.MaxWaitMillisecond, c.HookBatch.Deliver).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, converter.WithKey("option", err)
	}
	queue, err := converter.NewQueueConverter(c.Queue.Size).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, converter.WithKey("option", err)
	}
	run, err := converter.NewRunConverter(c.Run.Concurrency, c.Run.Interrupt).Convert()
	if err != nil {
		return domain.WatchTargetsOption{}, converter.WithKey("option", err)
	}
	return domain.WatchTargetsOption{
		Watch:     watch,
		DirBatch:  dirBatch,
		HookBatch: hookBatch,
		Queue:     queue,
		Run:       run,
	}, nil
}
