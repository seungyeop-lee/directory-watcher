package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/service"
)

// 이 파일의 테스트는 USAGE 두 판에 손으로 옮겨 적은 목록이 코드가 정의하는 목록과 같은지 확인한다.
// 실패하면 USAGE.md와 USAGE.ko.md의 해당 표와 예시를 코드 목록에 맞춰 함께 고친다.

// matchingRows는 모든 표에서 첫 칸이 pattern에 맞는 행마다, 첫 칸에서 pattern의 첫 묶음을 뽑고 나머지 칸과 함께 돌려준다.
func matchingRows(doc string, pattern *regexp.Regexp) (keys []string, rest [][]string) {
	for _, table := range markdownTables(doc) {
		for _, cells := range table.rows {
			if m := pattern.FindStringSubmatch(cells[0]); m != nil {
				keys = append(keys, m[1])
				rest = append(rest, cells[1:])
			}
		}
	}
	return keys, rest
}

func TestUsageListsSameHookVariablesAsCode(t *testing.T) {
	templates, envs := command.EventVariableNames()
	var want []string
	for i := range templates {
		want = append(want, templates[i]+" "+envs[i])
	}
	err := command.SingleCmd{Cmd: "{{.FilePath}}"}.CheckTemplates()
	if err == nil {
		t.Fatal("CheckTemplates accepted an unknown template variable")
	}
	for _, file := range usageFiles {
		doc := readDoc(t, file)
		names, rest := matchingRows(doc, regexp.MustCompile(`^\{\{\.(\w+)\}\}$`))
		var got []string
		for i, name := range names {
			got = append(got, name+" "+strings.Trim(rest[i][0], "`"))
		}
		if !sameSet(got, want) {
			t.Errorf("%s variable table = %q, want %q", file, got, want)
		}
		// 오류 예시 줄은 존재하지 않는 FilePath를 쓴 명령의 실제 오류 메시지를 옮겨 적은 것이다.
		if !strings.Contains(doc, err.Error()) {
			t.Errorf("%s has no error example %q", file, err.Error())
		}
	}
}

// 오류 예시 줄은 {{len .Path}}를 쓴 명령의 실제 오류 메시지를 옮겨 적은 것이다.
func TestUsageShowsUnsupportedTemplateActionError(t *testing.T) {
	err := command.SingleCmd{Cmd: "echo {{len .Path}}"}.CheckTemplates()
	if err == nil {
		t.Fatal("CheckTemplates accepted a function call")
	}
	for _, file := range usageFiles {
		if !strings.Contains(readDoc(t, file), err.Error()) {
			t.Errorf("%s has no error example %q", file, err.Error())
		}
	}
}

func TestUsageListsSameErrorVariablesAsCode(t *testing.T) {
	want := service.ErrorVariableNames()
	for _, file := range usageFiles {
		got, _ := matchingRows(readDoc(t, file), regexp.MustCompile("^`(DW_ERROR_\\w+)`$"))
		if !sameSet(got, want) {
			t.Errorf("%s DW_ERROR_* table = %q, want %q", file, got, want)
		}
	}
}

func TestUsageListsSameQueueWarningVariablesAsCode(t *testing.T) {
	want := service.QueueWarningVariableNames()
	for _, file := range usageFiles {
		got, _ := matchingRows(readDoc(t, file), regexp.MustCompile("^`(DW_QUEUE_\\w+)`$"))
		if !sameSet(got, want) {
			t.Errorf("%s DW_QUEUE_* table = %q, want %q", file, got, want)
		}
	}
}
