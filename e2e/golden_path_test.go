package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestGoldenPath(t *testing.T) {
	s := newScenario(t)
	s.start(goldenConfig(s))
	s.awaitLine("start hooks", 10*time.Second, "TARGET:onStartWatch")
	target := filepath.Join(s.watch, "foo.txt")
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s.awaitLine("create hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "C"))
	s.write(target, []byte("hello"))
	s.awaitLine("update hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "U"))
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	s.awaitLine("delete hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "D"))
	s.stop()
	expected := append(expectedSequence(target), "TARGET:onFinishWatch", "GLOBAL:onFinishWatch")
	actual := s.lines()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("hook sequence or seven event variables mismatch\nwant: %q\ngot: %q", expected, actual)
	}
}

// goldenConfig records every global and target hook with its label and, for
// change hooks, the seven event variables in changeValues order.
func goldenConfig(s *scenario) string {
	change := "{{.Path}}|{{.AbsPath}}|{{.FileName}}|{{.ExtName}}|{{.DirPath}}|{{.DirAbsPath}}|{{.Event}}"
	command := func(label, suffix string) string {
		if suffix != "" {
			label += ":" + suffix
		}
		return s.hook("--line=" + label)
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
      excludeSuffix: [.probe]
`, command("GLOBAL:onStartWatch", ""), command("GLOBAL:onBeforeChange", change), command("GLOBAL:onAfterChange", change), command("GLOBAL:onFinishWatch", ""), s.watch, command("TARGET:onStartWatch", ""), command("TARGET:onChange", change), command("TARGET:onFinishWatch", ""))
}

func changeValues(target, event string) string {
	dir := filepath.Dir(target)
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", target, target, filepath.Base(target), filepath.Ext(target), dir, dir, event)
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
