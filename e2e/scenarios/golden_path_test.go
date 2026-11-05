package scenarios

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestGoldenPath(t *testing.T) {
	t.Parallel()
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
		t.Fatalf("hook sequence or eight event variables mismatch\nwant: %q\ngot: %q", expected, actual)
	}
	for _, dir := range []string{s.Work, s.Watch} {
		if made, _ := filepath.Glob(filepath.Join(dir, "pwned*")); len(made) != 0 {
			t.Fatalf("file name ran as a hook command and made %q", made)
		}
	}
}
