package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestGoldenPath(t *testing.T) {
	s := harness.NewScenario(t)
	s.Start(goldenConfig(s))
	s.AwaitLine("start hooks", 10*time.Second, "TARGET:onStartWatch")
	target := filepath.Join(s.Watch, goldenFileName)
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s.AwaitLine("create hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "C"))
	s.Write(target, []byte("hello"))
	s.AwaitLine("update hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "U"))
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	s.AwaitLine("delete hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "D"))
	s.Stop()
	expected := append(expectedSequence(target), "TARGET:onFinishWatch", "GLOBAL:onFinishWatch")
	actual := s.Lines()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("hook sequence or seven event variables mismatch\nwant: %q\ngot: %q", expected, actual)
	}
	for _, dir := range []string{s.Work, s.Watch} {
		if made, _ := filepath.Glob(filepath.Join(dir, "pwned*")); len(made) != 0 {
			t.Fatalf("file name ran as a hook command and made %q", made)
		}
	}
}

// goldenFileName에는 sh, cmd, powershell이 명령을 잇거나 치환하는 문법이 들어 있다. 셸이 이벤트
// 변수 값을 명령으로 해석하면 pwned 폴더가 생기고, cmd가 값을 다시 펼치면 %OS%가 바뀐다.
const goldenFileName = `a b;mkdir pwned1;$(mkdir pwned2)'&mkdir pwned3&%OS%.txt`

// goldenConfig는 모든 전역 hook과 대상 hook이 라벨을 기록하게 하고, change hook은
// 일곱 이벤트 변수도 changeValues 순서대로 기록하게 한다. 변수는 따옴표 밖에 두어 값에 공백이
// 있어도 인자 하나로 넘어가게 하고, 구분자는 sh와 cmd가 해석하지 않는 쉼표를 쓴다.
func goldenConfig(s *harness.Scenario) string {
	change := "{{.Path}},{{.AbsPath}},{{.Name}},{{.ExtName}},{{.ParentPath}},{{.ParentAbsPath}},{{.Event}}"
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
      waitMillisecond: 100
      watchEvent: CDU
      exclude: ['*.probe']
`, command("GLOBAL:onStartWatch", ""), command("GLOBAL:onBeforeChange", change), command("GLOBAL:onAfterChange", change), command("GLOBAL:onFinishWatch", ""), s.Watch, command("TARGET:onStartWatch", ""), command("TARGET:onChange", change), command("TARGET:onFinishWatch", ""))
}

func changeValues(target, event string) string {
	dir := filepath.Dir(target)
	return fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s", target, target, filepath.Base(target), filepath.Ext(target), dir, dir, event)
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
