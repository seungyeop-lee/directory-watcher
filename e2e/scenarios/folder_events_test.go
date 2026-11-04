package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// TestFolderEvents는 사용자 동작마다 hook이 받는 이벤트를 {{.Event}},{{.IsDir}},<감시 폴더 기준 경로>로 확인한다.
// 폴더가 안의 내용과 함께 생기거나 사라지면 맨 위 폴더의 이벤트 하나만 온다.
func TestFolderEvents(t *testing.T) {
	s := harness.NewScenario(t)
	outside := filepath.Join(s.Work, "outside")
	for _, dir := range []string{filepath.Join(s.Watch, "src"), filepath.Join(s.Watch, "dst"), outside} {
		mkdirAll(t, dir)
	}
	// Windows는 감시 중인 하위 폴더가 있는 폴더의 이름을 바꾸거나 옮기지 못하므로, 옮기는 폴더에는 파일만 둔다.
	fill(t, filepath.Join(outside, "x"), 3)
	fill(t, filepath.Join(outside, "big"), 300)
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
      noWait: true
`, s.Watch, s.Hook("--line=CHANGE:{{.Event}},{{.IsDir}},{{.Path}}")))

	watch := func(rel string) string { return filepath.Join(s.Watch, filepath.FromSlash(rel)) }
	move := func(from, to string) {
		t.Helper()
		if err := os.Rename(from, to); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct {
		name string
		act  func()
		want []string
	}{
		{"mkdir", func() { mkdirAll(t, watch("src/pkg")) }, []string{"C,true,src/pkg"}},
		{"mkdir -p", func() { mkdirAll(t, watch("src/a/b/c")) }, []string{"C,true,src/a"}},
		{"move in a folder with files", func() { move(filepath.Join(outside, "x"), watch("src/x")) }, []string{"C,true,src/x"}},
		{"mkdir and write a file in it", func() {
			mkdirAll(t, watch("src/n"))
			s.Write(watch("src/n/a.txt"), []byte("hi"))
		}, []string{"C,true,src/n"}},
		{"move in a folder with 300 files", func() { move(filepath.Join(outside, "big"), watch("src/big")) }, []string{"C,true,src/big"}},
		{"rm -rf a folder with 300 files", func() {
			if err := os.RemoveAll(watch("src/big")); err != nil {
				t.Fatal(err)
			}
		}, []string{"D,true,src/big"}},
		{"move a folder out of the target", func() { move(watch("src/x"), filepath.Join(outside, "x")) }, []string{"D,true,src/x"}},
		{"rename a folder", func() { move(watch("src/n"), watch("src/m")) }, []string{"D,true,src/n", "C,true,src/m"}},
		{"move a folder under another folder", func() { move(watch("src/m"), watch("dst/m")) }, []string{"D,true,src/m", "C,true,dst/m"}},
		{"create a file", func() { createEmpty(t, watch("src/f.txt")) }, []string{"C,false,src/f.txt"}},
		{"rename a file", func() { move(watch("src/f.txt"), watch("src/g.txt")) }, []string{"D,false,src/f.txt", "C,false,src/g.txt"}},
		{"delete a file", func() {
			if err := os.Remove(watch("src/g.txt")); err != nil {
				t.Fatal(err)
			}
		}, []string{"D,false,src/g.txt"}},
	}
	seen := 0
	for i, step := range steps {
		step.act()
		// Windows는 폴더마다 이벤트를 따로 받으므로, 이벤트가 생기는 폴더마다 표지 파일을 만들어 모두 기다린다.
		var sentinels []string
		for _, dir := range []string{"", "src", "dst"} {
			rel := strings.TrimPrefix(fmt.Sprintf("%s/zz-sentinel-%d.txt", dir, i), "/")
			createEmpty(t, watch(rel))
			sentinels = append(sentinels, "C,false,"+rel)
		}
		var got []string
		s.Await(step.name+" sentinels", 10*time.Second, func() bool {
			got = nil
			waiting := map[string]bool{}
			for _, sentinel := range sentinels {
				waiting[sentinel] = true
			}
			lines := s.Lines()
			for _, line := range lines[seen:] {
				event := relativeChange(t, s.Watch, line)
				if waiting[event] {
					delete(waiting, event)
				} else if !strings.Contains(event, "zz-sentinel-") {
					got = append(got, event)
				}
			}
			if len(waiting) > 0 {
				return false
			}
			seen = len(lines)
			return true
		})
		if !reflect.DeepEqual(got, step.want) {
			t.Errorf("%s: hook received %q, want %q", step.name, got, step.want)
		}
	}
	s.Stop()
}

// relativeChange는 CHANGE:<Event>,<IsDir>,<절대 경로> 기록을 <Event>,<IsDir>,<감시 폴더 기준 경로>로 바꾼다.
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

// fill은 dir 안에 파일 n개를 만든다.
func fill(t *testing.T, dir string, n int) {
	t.Helper()
	mkdirAll(t, dir)
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
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
