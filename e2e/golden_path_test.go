package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGoldenPath(t *testing.T) {
	s := newScenario(t)
	s.start(buildConfig(hookrecBin, s.records, s.watch))
	s.waitLines(2)
	target := filepath.Join(s.watch, "foo.txt")
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s.waitLines(5)
	s.write(target, []byte("hello"))
	s.waitLines(8)
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	s.waitLines(11)
	s.stop()
	expected := expectedSequence(target)
	if canVerifyFinishHooks {
		expected = append(expected, "TARGET:onFinishWatch", "GLOBAL:onFinishWatch")
	}
	actual := readLinesLenient(s.records)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("hook sequence or seven event variables mismatch\nwant: %q\ngot: %q", expected, actual)
	}
}

func buildConfig(hookrec, logFile, watchDir string) string {
	change := "{{.Path}}|{{.AbsPath}}|{{.FileName}}|{{.ExtName}}|{{.DirPath}}|{{.DirAbsPath}}|{{.Event}}"
	command := func(label, suffix string) string {
		if suffix != "" {
			label += ":" + suffix
		}
		return fmt.Sprintf("'%s --file=%s --line=%s'", hookrec, logFile, label)
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
`, command("GLOBAL:onStartWatch", ""), command("GLOBAL:onBeforeChange", change), command("GLOBAL:onAfterChange", change), command("GLOBAL:onFinishWatch", ""), watchDir, command("TARGET:onStartWatch", ""), command("TARGET:onChange", change), command("TARGET:onFinishWatch", ""))
}

func expectedSequence(target string) []string {
	dir := filepath.Dir(target)
	result := []string{"GLOBAL:onStartWatch", "TARGET:onStartWatch"}
	for _, event := range []string{"C", "U", "D"} {
		values := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", target, target, filepath.Base(target), filepath.Ext(target), dir, dir, event)
		for _, hook := range []string{"GLOBAL:onBeforeChange", "TARGET:onChange", "GLOBAL:onAfterChange"} {
			result = append(result, hook+":"+values)
		}
	}
	return result
}
