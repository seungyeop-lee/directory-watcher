package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/service"
)

// 이 파일의 테스트는 docs/guide 문서 두 판에 손으로 옮겨 적은 목록이 코드가 정의하는 목록과 같은지 확인한다.
// 실패하면 그 문서의 영어판과 한국어판에서 해당 표와 예시를 코드 목록에 맞춰 함께 고친다.

func TestGuideListsSameHookVariablesAsCode(t *testing.T) {
	templates, envs := command.EventVariableNames()
	var want []string
	for i := range templates {
		want = append(want, templates[i]+" "+envs[i])
	}
	err := command.SingleCmd{Cmd: "{{.FilePath}}"}.CheckTemplates()
	if err == nil {
		t.Fatal("CheckTemplates accepted an unknown template variable")
	}
	for _, file := range guideFiles("configuration") {
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
func TestGuideShowsUnsupportedTemplateActionError(t *testing.T) {
	err := command.SingleCmd{Cmd: "echo {{len .Path}}"}.CheckTemplates()
	if err == nil {
		t.Fatal("CheckTemplates accepted a function call")
	}
	for _, file := range guideFiles("configuration") {
		if !strings.Contains(readDoc(t, file), err.Error()) {
			t.Errorf("%s has no error example %q", file, err.Error())
		}
	}
}

func TestGuideListsSameErrorVariablesAsCode(t *testing.T) {
	want := service.ErrorVariableNames()
	for _, file := range guideFiles("running") {
		got, _ := matchingRows(readDoc(t, file), regexp.MustCompile("^`(DW_ERROR_\\w+)`$"))
		if !sameSet(got, want) {
			t.Errorf("%s DW_ERROR_* table = %q, want %q", file, got, want)
		}
	}
}

func TestGuideListsSameQueueWarningVariablesAsCode(t *testing.T) {
	want := service.QueueWarningVariableNames()
	for _, file := range guideFiles("running") {
		got, _ := matchingRows(readDoc(t, file), regexp.MustCompile("^`(DW_QUEUE_\\w+)`$"))
		if !sameSet(got, want) {
			t.Errorf("%s DW_QUEUE_* table = %q, want %q", file, got, want)
		}
	}
}
