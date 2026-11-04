package domain

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExcludePatterns_Excludes(t *testing.T) {
	tests := []struct {
		name     string
		patterns ExcludePatterns
		excluded []string
		watched  []string
	}{
		{
			name:     "file name matches only from the target root",
			patterns: ExcludePatterns{"out.txt"},
			excluded: []string{"out.txt"},
			watched:  []string{"layout.txt", "sub/out.txt", "out.txt.bak"},
		},
		{
			name:     "extension at any depth",
			patterns: ExcludePatterns{"**/*.log"},
			excluded: []string{"a.log", "sub/deep/b.log"},
			watched:  []string{"a.txt", "a.log.txt"},
		},
		{
			name:     "pattern with a sub path",
			patterns: ExcludePatterns{"build/**/*.o"},
			excluded: []string{"build/x.o", "build/sub/y.o"},
			watched:  []string{"build", "build/x.c", "src/x.o"},
		},
		{
			name:     "folder and everything below it",
			patterns: ExcludePatterns{"dist/**"},
			excluded: []string{"dist", "dist/a.js", "dist/sub/b.js"},
			watched:  []string{"distx/a.js", "src/dist/a.js"},
		},
		{
			name:     "negation re-includes a file excluded by an earlier pattern",
			patterns: ExcludePatterns{"**/*.log", "!important.log"},
			excluded: []string{"a.log", "sub/important.log"},
			watched:  []string{"important.log"},
		},
		{
			name:     "the last matching pattern decides",
			patterns: ExcludePatterns{"!important.log", "**/*.log"},
			excluded: []string{"important.log"},
		},
		{
			name:     "negation re-includes a file inside a folder that is not excluded",
			patterns: ExcludePatterns{"dist/*", "!dist/keep.txt"},
			excluded: []string{"dist/a.txt"},
			watched:  []string{"dist", "dist/keep.txt"},
		},
		{
			name:     "negation cannot re-include a file inside an excluded folder",
			patterns: ExcludePatterns{"dist/**", "!dist/keep.txt"},
			excluded: []string{"dist", "dist/a.txt", "dist/keep.txt"},
		},
		{
			name:    "no patterns",
			watched: []string{"a.txt", "sub/b.txt"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, rel := range tt.excluded {
				if !tt.patterns.Excludes(rel) {
					t.Errorf("Excludes(%q) = false, want true", rel)
				}
			}
			for _, rel := range tt.watched {
				if tt.patterns.Excludes(rel) {
					t.Errorf("Excludes(%q) = true, want false", rel)
				}
			}
		})
	}
}

// USAGE "Moving from excludeDir and excludeSuffix"의 각 행에서, 옛 옵션이 빼던 경로와 exclude 패턴이 빼는
// 경로가 같은지 확인한다. 감시 대상 path는 site다.
func TestExcludePatternsMatchRemovedOptions(t *testing.T) {
	const target = "site"
	// 옛 excludeDir는 실행 위치 기준 폴더 경로였고, 그 폴더와 그 아래를 감시 목록에 넣지 않았다.
	excludeDir := func(dir string) func(rel string) bool {
		return func(rel string) bool { return Path(dir).Contains(Path(target + "/" + rel)) }
	}
	// 옛 excludeSuffix는 이벤트 경로의 절대 경로 끝부분을 비교했다.
	excludeSuffix := func(suffix string) func(rel string) bool {
		return func(rel string) bool {
			return strings.HasSuffix(filepath.ToSlash(Path(target+"/"+rel).Abs()), suffix)
		}
	}
	paths := []string{
		"dist", "dist/a.js", "dist/sub/b.js", "distx/a.js", "src/dist/a.js",
		"a_test.go", "pkg/b_test.go", "test.go", "a_test.go.bak",
		"a.txt~", "sub/b~", "a~.txt",
		"out.txt", "sub/out.txt", "layout.txt", "out.txt.bak",
	}
	rows := []struct {
		old      string
		removed  func(rel string) bool
		patterns ExcludePatterns
	}{
		{old: "excludeDir: ['site/dist']", removed: excludeDir("site/dist"), patterns: ExcludePatterns{"dist/**"}},
		{old: "excludeSuffix: ['_test.go']", removed: excludeSuffix("_test.go"), patterns: ExcludePatterns{"**/*_test.go"}},
		{old: "excludeSuffix: ['~']", removed: excludeSuffix("~"), patterns: ExcludePatterns{"**/*~"}},
		{old: "excludeSuffix: ['/out.txt']", removed: excludeSuffix("/out.txt"), patterns: ExcludePatterns{"**/out.txt"}},
	}
	for _, row := range rows {
		t.Run(row.old, func(t *testing.T) {
			for _, rel := range paths {
				if old, got := row.removed(rel), row.patterns.Excludes(rel); old != got {
					t.Errorf("%s: old option excludes = %v, exclude %q excludes = %v", rel, old, row.patterns, got)
				}
			}
		})
	}
}

func TestParseExcludePatterns(t *testing.T) {
	if _, err := ParseExcludePatterns([]string{"**/*.log", "!important.log"}); err != nil {
		t.Fatalf("valid patterns: %v", err)
	}
	_, err := ParseExcludePatterns([]string{"ok.txt", "![a"})
	if err == nil || err.Error() != `exclude[1]: invalid pattern "![a"` {
		t.Fatalf("invalid pattern error = %v", err)
	}
}
