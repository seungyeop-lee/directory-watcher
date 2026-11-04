package yaml

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// checkRemovedKeys는 v3에서 없애거나 그룹 안으로 옮긴 키가 설정에 있으면, 대신 쓸 키와 USAGE의 해당 절을 알리는 오류를 낸다.
// 키에 값을 적지 않았거나 빈 목록을 적어도 키가 있으면 오류다.
func (c WatchTargetsOptionConfig) checkRemovedKeys() error {
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
		{"interruptible", "option.run.interrupt: oldest in place of interruptible: true, or option.run.interrupt: none in place of interruptible: false", optionsSection, c.RemovedInterruptible},
	} {
		if removed.node.Kind != 0 {
			return fmt.Errorf(`%s is no longer supported; use %s (see USAGE %q)`, removed.key, removed.replacement, removed.section)
		}
	}
	return nil
}
