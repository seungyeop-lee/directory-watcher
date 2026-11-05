package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// changeSteps는 hook이 CHANGE:{{.Event}},{{.IsDir}},{{.Path}}로 남긴 기록을 사용자 동작 단계마다 나눠 읽는다.
type changeSteps struct {
	t *testing.T
	s *harness.Scenario
	// root는 표지를 만들 감시 폴더이고, 기록의 경로는 이 폴더 기준 경로로 바꾼다.
	root string
	// dirSentinels이면 표지를 빈 파일 대신 폴더로 만든다. 파일 이벤트가 hook을 실행하지 않는 설정에서 쓴다.
	dirSentinels bool
	// timeout은 next가 표지와 기다리는 기록을 기다리는 가장 긴 시간이고, 0이면 10초다.
	timeout    time.Duration
	seen, step int
}

// next는 root 아래 dirs의 각 폴더에 표지를 만들고, hook이 표지를 모두 받고 awaited의 기록도 모두 받을 때까지
// 기다린 뒤, 앞 단계 이후 받은 표지 아닌 기록을 <Event>,<IsDir>,<root 기준 경로>로 돌려준다. 운영체제는 한
// 폴더의 이벤트를 일어난 순서대로 알리므로, 표지보다 먼저 한 동작이 hook을 실행했다면 그 기록이 함께 돌아온다.
// 그래서 표지를 만들 수 없는 폴더의 기록은 awaited로 기다린다.
func (c *changeSteps) next(label string, awaited []string, dirs ...string) []string {
	c.t.Helper()
	var sentinels []string
	for _, dir := range dirs {
		rel := strings.TrimPrefix(fmt.Sprintf("%s/zz-sentinel-%d", dir, c.step), "/")
		if c.dirSentinels {
			mkdirAll(c.t, filepath.Join(c.root, filepath.FromSlash(rel)))
			sentinels = append(sentinels, "C,true,"+rel)
		} else {
			rel += ".txt"
			createEmpty(c.t, filepath.Join(c.root, filepath.FromSlash(rel)))
			sentinels = append(sentinels, "C,false,"+rel)
		}
	}
	c.step++
	timeout := c.timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	var got []string
	c.s.Await(label+" sentinels", timeout, func() bool {
		got = nil
		waiting := map[string]bool{}
		for _, sentinel := range sentinels {
			waiting[sentinel] = true
		}
		lines := c.s.Lines()
		for _, line := range lines[c.seen:] {
			event := relativeChange(c.t, c.root, line)
			if waiting[event] {
				delete(waiting, event)
			} else if !strings.Contains(event, "zz-sentinel-") {
				got = append(got, event)
			}
		}
		if len(waiting) > 0 {
			return false
		}
		for _, record := range awaited {
			if !slices.Contains(got, record) {
				return false
			}
		}
		c.seen = len(lines)
		return true
	})
	return got
}

// relativeChange는 CHANGE:<Event>,<IsDir>,<절대 경로> 기록을 <Event>,<IsDir>,<watch 기준 경로>로 바꾼다.
func relativeChange(t *testing.T, watch, line string) string {
	t.Helper()
	parts := strings.SplitN(strings.TrimPrefix(line, "CHANGE:"), ",", 3)
	if len(parts) != 3 {
		t.Fatalf("unexpected record %q", line)
	}
	rel, err := filepath.Rel(watch, parts[2])
	if err != nil {
		t.Fatal(err)
	}
	return parts[0] + "," + parts[1] + "," + filepath.ToSlash(rel)
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
}

// createEmpty는 내용 없는 파일을 만든다. 내용을 쓰지 않으므로 운영체제와 관계없이 생성 이벤트만 생긴다.
func createEmpty(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// goldenFileName에는 sh, cmd, powershell이 명령을 잇거나 치환하는 문법이 들어 있다. 셸이 이벤트
// 변수 값을 명령으로 해석하면 pwned 폴더가 생기고, cmd가 값을 다시 펼치면 %OS%가 바뀐다.
const goldenFileName = `a b;mkdir pwned1;$(mkdir pwned2)'&mkdir pwned3&%OS%.txt`

// goldenConfig는 모든 전역 hook과 대상 hook이 라벨을 기록하게 하고, change hook은
// 여덟 이벤트 변수도 changeValues 순서대로 기록하게 한다. 변수는 따옴표 밖에 두어 값에 공백이
// 있어도 인자 하나로 넘어가게 하고, 구분자는 sh와 cmd가 해석하지 않는 쉼표를 쓴다.
func goldenConfig(s *harness.Scenario) string {
	change := "{{.Path}},{{.AbsPath}},{{.Name}},{{.ExtName}},{{.ParentPath}},{{.ParentAbsPath}},{{.Event}},{{.IsDir}}"
	command := func(label, suffix string) string {
		if suffix != "" {
			label += ":" + suffix
		}
		return s.Hook("--line=" + label)
	}
	return fmt.Sprintf(`global:
  lifeCycle:
    onStartWatch:
      - %s
    onBeforeChange:
      - %s
    onAfterChange:
      - %s
    onFinishWatch:
      - %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: CDU
      hookBatch:
        waitMillisecond: 100
`, command("GLOBAL:onStartWatch", ""), command("GLOBAL:onBeforeChange", change), command("GLOBAL:onAfterChange", change), command("GLOBAL:onFinishWatch", ""), s.Watch, command("TARGET:onStartWatch", ""), command("TARGET:onChange", change), command("TARGET:onFinishWatch", ""))
}

func changeValues(target, event string) string {
	dir := filepath.Dir(target)
	return fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,false", target, target, filepath.Base(target), filepath.Ext(target), dir, dir, event)
}

func expectedSequence(target string) []string {
	result := []string{"GLOBAL:onStartWatch", "TARGET:onStartWatch"}
	for _, event := range []string{"C", "U", "D"} {
		for _, hook := range []string{"GLOBAL:onBeforeChange", "TARGET:onChange", "GLOBAL:onAfterChange"} {
			result = append(result, hook+":"+changeValues(target, event))
		}
	}
	return result
}
