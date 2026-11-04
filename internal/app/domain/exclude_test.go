package domain

import "testing"

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

func TestParseExcludePatterns(t *testing.T) {
	if _, err := ParseExcludePatterns([]string{"**/*.log", "!important.log"}); err != nil {
		t.Fatalf("valid patterns: %v", err)
	}
	_, err := ParseExcludePatterns([]string{"ok.txt", "![a"})
	if err == nil || err.Error() != `exclude[1]: invalid pattern "![a"` {
		t.Fatalf("invalid pattern error = %v", err)
	}
}
