package main

import (
	"regexp"
	"strings"
	"testing"
)

// 이 테스트는 설정 예시 첫 줄의 스키마 URL이 go.mod 모듈 경로의 major 브랜치를 가리키는지 확인한다.
// 실패하면 아래 파일들에서 `# yaml-language-server: $schema=` 줄의 v<major>를 모듈 경로의 major에 맞춰 함께 고친다.

// schemaURLFiles는 스키마 URL 줄을 넣은 파일과 그 줄의 수다.
var schemaURLFiles = map[string]int{
	exampleConfigFile:                    1,
	"README.md":                          2,
	"README.ko.md":                       2,
	guideFile("configuration", ".md"):    1,
	guideFile("configuration", ".ko.md"): 1,
}

func TestConfigExamplesLinkSchemaOfModuleMajor(t *testing.T) {
	module := regexp.MustCompile(`(?m)^module \S+/v(\d+)$`).FindStringSubmatch(readDoc(t, "go.mod"))
	if module == nil {
		t.Fatal("go.mod has no module path with a /v<major> suffix")
	}
	want := "# yaml-language-server: $schema=https://raw.githubusercontent.com/seungyeop-lee/directory-watcher/v" +
		module[1] + "/" + schemaFile
	line := regexp.MustCompile(`(?m)^\s*(# yaml-language-server:.*)$`)
	for file, count := range schemaURLFiles {
		doc := readDoc(t, file)
		matches := line.FindAllStringSubmatch(doc, -1)
		if len(matches) != count {
			t.Errorf("%s has %d schema URL lines, want %d", file, len(matches), count)
		}
		for _, m := range matches {
			if m[1] != want {
				t.Errorf("%s schema URL line = %q, want %q", file, m[1], want)
			}
		}
	}
	if first, _, _ := strings.Cut(readDoc(t, exampleConfigFile), "\n"); first != want {
		t.Errorf("%s first line = %q, want %q", exampleConfigFile, first, want)
	}
}
