package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// 이 파일의 테스트는 e2e 문서에 손으로 적은 시나리오 목록이 e2e/scenarios의 시나리오와 맞는지 확인한다.
// 목록을 넣는 곳과 조건은 e2e/docs/harness.md "Writing a new scenario"의 5–7단계가 정한다.
// 실패하면 보고된 목록에 시나리오 이름을 넣거나 빼고, 한국어판과 영어판을 함께 고친다.

const (
	e2eScenariosDir = "e2e/scenarios"
	e2eDocsDir      = "e2e/docs/"
	ciWorkflowFile  = ".github/workflows/ci.yml"
)

// e2eDocLanguage는 e2e 문서 한 판에서 시나리오 목록을 찾는 데 쓰는 확장자와 제목이다.
type e2eDocLanguage struct {
	ext string
	// readmeRow는 e2e/README의 세부 문서 표에서 시나리오 문서 링크를 모은 행의 첫 칸 머리말이다.
	readmeRow             string
	raceSection           string
	lossSection           string
	windowsRunSection     string
	windowsCIStepsSection string
	windowsCIResult       string
	utmSelectionSection   string
}

var e2eDocLanguages = []e2eDocLanguage{
	{
		ext:                   ".md",
		readmeRow:             "Per-scenario documents: ",
		raceSection:           "## Race and overload verification",
		lossSection:           "## Event-loss notification verification",
		windowsRunSection:     "## Run commands",
		windowsCIStepsSection: "## CI steps",
		windowsCIResult:       "## Checking CI results",
		utmSelectionSection:   "### Test selection, repetition, and time limits",
	},
	{
		ext:                   ".ko.md",
		readmeRow:             "시나리오별 문서: ",
		raceSection:           "## 경합과 과부하 검증",
		lossSection:           "## 이벤트 유실 알림 검증",
		windowsRunSection:     "## 실행 명령",
		windowsCIStepsSection: "## CI 단계",
		windowsCIResult:       "## CI 결과 확인",
		utmSelectionSection:   "### 테스트 선택·반복·시간 제한",
	},
}

// scenarioList는 문서나 ci.yml의 한 곳에 적힌 시나리오 이름 목록이다.
type scenarioList struct {
	location string
	names    []string
}

var scenarioFunc = regexp.MustCompile(`(?m)^func (Test\w+)\(\w+ \*testing\.T\)`)

// e2eScenarioNames는 e2e/scenarios/*_test.go의 최상위 Test 함수 이름을 소스 텍스트에서 모은다.
// windows_cancel_test.go처럼 빌드 태그가 붙은 파일의 시나리오도 OS와 관계없이 모으려고 컴파일하지 않고 읽는다.
func e2eScenarioNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(e2eScenariosDir, "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, file := range files {
		for _, m := range scenarioFunc.FindAllStringSubmatch(readDoc(t, file), -1) {
			names = append(names, m[1])
		}
	}
	if len(names) == 0 {
		t.Fatalf("no scenario test functions in %s", e2eScenariosDir)
	}
	return names
}

// scenarioDocName은 harness.md 5단계의 규칙대로 시나리오 이름에서 Test를 떼고 kebab-case로 바꾼 문서 이름이다.
// 대문자가 이어지는 약어는 한 단어로 본다(TestBoundedOverloadCLI → bounded-overload-cli).
func scenarioDocName(name string) string {
	word := []rune(strings.TrimPrefix(name, "Test"))
	var b strings.Builder
	for i, r := range word {
		if i > 0 && unicode.IsUpper(r) {
			acronymEnds := i+1 < len(word) && unicode.IsLower(word[i+1])
			if !unicode.IsUpper(word[i-1]) || acronymEnds {
				b.WriteByte('-')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// scenarioDocNames는 e2e/docs/scenarios에서 확장자가 ext인 문서의 이름을 확장자 없이 모은다.
func scenarioDocNames(t *testing.T, ext string) []string {
	t.Helper()
	entries, err := os.ReadDir(e2eDocsDir + "scenarios")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		name, found := strings.CutSuffix(entry.Name(), ext)
		// 영어판 확장자 .md는 한국어판 .ko.md의 끝과도 맞으므로, 한국어판을 따로 빼야 한다.
		if found && !strings.HasSuffix(name, ".ko") {
			names = append(names, name)
		}
	}
	return names
}

var scenarioLink = regexp.MustCompile("\\[`(Test\\w+)`\\]\\(([^)#]*)[^)]*\\)")

// linkedScenarios는 text에서 [`Test...`](...) 링크의 시나리오 이름을 모은다.
// 링크가 같은 판(ext)의 그 시나리오 문서를 가리키지 않으면 file과 함께 알린다.
func linkedScenarios(t *testing.T, file, ext, text string) []string {
	t.Helper()
	var names []string
	for _, m := range scenarioLink.FindAllStringSubmatch(text, -1) {
		names = append(names, m[1])
		if want := "scenarios/" + scenarioDocName(m[1]) + ext; !strings.HasSuffix(m[2], want) {
			t.Errorf("%s: link of %s points to %q, want a path ending in %q", file, m[1], m[2], want)
		}
	}
	return names
}

var mentionedScenario = regexp.MustCompile("`(Test\\w+)`")

// mentionedScenarios는 text에서 백틱으로 감싼 Test 이름을 링크 글자까지 모두 모은다.
func mentionedScenarios(text string) []string {
	var names []string
	for _, m := range mentionedScenario.FindAllStringSubmatch(text, -1) {
		names = append(names, m[1])
	}
	return names
}

var (
	runFlag      = regexp.MustCompile(`-run '([^']*)'`)
	scenarioName = regexp.MustCompile(`^Test\w+$`)
)

// runPatterns는 text에 나오는 go test -run 정규식을 차례대로 시나리오 이름 목록으로 바꾼다.
// 정규식은 'A|B'와 '^(A|B)$', '^A$' 꼴만 읽고, 다른 꼴이면 file과 함께 알린다.
func runPatterns(t *testing.T, file, text string) [][]string {
	t.Helper()
	var patterns [][]string
	for _, m := range runFlag.FindAllStringSubmatch(text, -1) {
		alternatives := strings.TrimSuffix(strings.TrimPrefix(m[1], "^"), "$")
		alternatives = strings.TrimSuffix(strings.TrimPrefix(alternatives, "("), ")")
		names := strings.Split(alternatives, "|")
		for _, name := range names {
			if !scenarioName.MatchString(name) {
				t.Errorf("%s: cannot read scenario names from -run %q", file, m[1])
			}
		}
		patterns = append(patterns, names)
	}
	return patterns
}

// firstRunPattern은 text의 첫 -run 정규식이 담은 시나리오 이름이다. 정규식이 없으면 file과 함께 알린다.
func firstRunPattern(t *testing.T, file, text string) []string {
	t.Helper()
	patterns := runPatterns(t, file, text)
	if len(patterns) == 0 {
		t.Errorf("%s: no -run pattern", file)
		return nil
	}
	return patterns[0]
}

var markdownHeading = regexp.MustCompile(`^#{1,6} `)

// markdownSection은 doc에서 heading 줄 바로 다음부터 다음 제목 줄 앞까지를 돌려준다. 코드 블록 안의 # 줄은 제목으로 보지 않는다.
func markdownSection(t *testing.T, file, doc, heading string) string {
	t.Helper()
	_, rest, found := strings.Cut(doc, "\n"+heading+"\n")
	if !found {
		t.Fatalf("%s has no section %q", file, heading)
	}
	var section []string
	inCode := false
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
		}
		if !inCode && markdownHeading.MatchString(line) {
			break
		}
		section = append(section, line)
	}
	return strings.Join(section, "\n")
}

// markdownCode는 section에 있는 코드 블록의 줄만 모은다.
// 목록의 -run 정규식은 코드 블록의 명령에 있고, 본문 bullet의 -run은 다른 패키지를 설명하는 예일 수 있다.
func markdownCode(section string) string {
	var code []string
	inCode := false
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
		} else if inCode {
			code = append(code, line)
		}
	}
	return strings.Join(code, "\n")
}

// ciStep은 ci.yml에서 이름이 name인 step의 본문을 다음 step 앞까지 돌려준다.
func ciStep(t *testing.T, workflow, name string) string {
	t.Helper()
	_, rest, found := strings.Cut(workflow, "- name: "+name+"\n")
	if !found {
		t.Fatalf("%s has no step %q", ciWorkflowFile, name)
	}
	step, _, _ := strings.Cut(rest, "\n      - ")
	return step
}

// checkSameScenarioLists는 lists의 각 목록이 첫 목록과 다르면, 첫 목록에만 있는 이름과 그 목록에만 있는 이름을 알린다.
func checkSameScenarioLists(t *testing.T, subset string, lists []scenarioList) {
	t.Helper()
	reference := lists[0]
	for _, list := range lists[1:] {
		if missing, extra := setDifference(list.names, reference.names); len(missing) > 0 || len(extra) > 0 {
			t.Errorf("%s list %s: missing %q, extra %q compared with %s", subset, list.location, missing, extra, reference.location)
		}
	}
}

func TestE2EDocsListEveryScenario(t *testing.T) {
	scenarios := e2eScenarioNames(t)
	var docNames []string
	for _, name := range scenarios {
		docNames = append(docNames, scenarioDocName(name))
	}
	check := func(location string, got, want []string) {
		t.Helper()
		if missing, extra := setDifference(got, want); len(missing) > 0 || len(extra) > 0 {
			t.Errorf("%s: missing scenarios %q, names not in %s %q", location, missing, e2eScenariosDir, extra)
		}
	}
	for _, lang := range e2eDocLanguages {
		harness := e2eDocsDir + "harness" + lang.ext
		// 시나리오 표에서 첫 칸에 링크가 없는 행은 e2e/harness의 Windows 전용 테스트라서 목록에 넣지 않는다.
		cells, _ := matchingRows(readDoc(t, harness), regexp.MustCompile("^(\\[`Test.*)$"))
		check(harness+" scenario table", linkedScenarios(t, harness, lang.ext, strings.Join(cells, "\n")), scenarios)

		readme := "e2e/README" + lang.ext
		row, _ := matchingRows(readDoc(t, readme), regexp.MustCompile("^"+regexp.QuoteMeta(lang.readmeRow)+"(.*)$"))
		check(readme+" "+strings.TrimSpace(lang.readmeRow)+" row", linkedScenarios(t, readme, lang.ext, strings.Join(row, "\n")), scenarios)

		check(e2eDocsDir+"scenarios/*"+lang.ext, scenarioDocNames(t, lang.ext), docNames)
	}
}

// 시나리오 일부만 담는 목록은 같은 부분 집합을 담는 목록끼리만 비교한다. 어떤 시나리오가 그 부분 집합에 속해야 하는지는 판정하지 않는다.
func TestE2EDocsSubsetListsAgree(t *testing.T) {
	workflow := readDoc(t, ciWorkflowFile)
	lifecycleStep := ciStep(t, workflow, "Repeat Windows graceful lifecycle without test cache")
	notificationStep := ciStep(t, workflow, "Verify Windows error notification process trees")
	var race, loss, raceStep []scenarioList
	lifecycle := []scenarioList{{ciWorkflowFile + " graceful lifecycle step", firstRunPattern(t, ciWorkflowFile, lifecycleStep)}}
	notification := []scenarioList{{ciWorkflowFile + " notification process tree step", firstRunPattern(t, ciWorkflowFile, notificationStep)}}

	for _, lang := range e2eDocLanguages {
		reliability := e2eDocsDir + "reliability" + lang.ext
		doc := readDoc(t, reliability)
		for _, subset := range []struct {
			heading string
			lists   *[]scenarioList
		}{{lang.raceSection, &race}, {lang.lossSection, &loss}} {
			section := markdownSection(t, reliability, doc, subset.heading)
			patterns := runPatterns(t, reliability, markdownCode(section))
			if len(patterns) != 2 {
				t.Errorf("%s %q: %d -run patterns, want the go test and e2e/linux commands", reliability, subset.heading, len(patterns))
			}
			for i, names := range patterns {
				*subset.lists = append(*subset.lists, scenarioList{fmt.Sprintf("%s %s command %d", reliability, subset.heading, i+1), names})
			}
			*subset.lists = append(*subset.lists, scenarioList{reliability + " " + subset.heading + " links", linkedScenarios(t, reliability, lang.ext, section)})
		}

		windows := e2eDocsDir + "windows" + lang.ext
		doc = readDoc(t, windows)
		run := markdownSection(t, windows, doc, lang.windowsRunSection)
		lifecycle = append(lifecycle,
			scenarioList{windows + " " + lang.windowsRunSection + " command", firstRunPattern(t, windows, markdownCode(run))},
			scenarioList{windows + " " + lang.windowsRunSection + " links", linkedScenarios(t, windows, lang.ext, run)})
		// CI 단계 절은 정상 종료 반복 시나리오를 이름 없이 "위 세 테스트"로만 가리키므로, 이 절의 시나리오 이름은 알림 트리 검증 시나리오다.
		steps := markdownSection(t, windows, doc, lang.windowsCIStepsSection)
		linkedScenarios(t, windows, lang.ext, steps) // 링크가 같은 판의 문서를 가리키는지만 확인한다.
		notification = append(notification, scenarioList{windows + " " + lang.windowsCIStepsSection, mentionedScenarios(steps)})
		var raceStepNames []string
		for _, line := range strings.Split(markdownSection(t, windows, doc, lang.windowsCIResult), "\n") {
			if strings.HasPrefix(line, "- ") && strings.Contains(line, "`go test -count=1 -race ./...`") {
				linkedScenarios(t, windows, lang.ext, line)
				raceStepNames = append(raceStepNames, mentionedScenarios(line)...)
			}
		}
		raceStep = append(raceStep, scenarioList{windows + " " + lang.windowsCIResult + " -race step", raceStepNames})

		utm := e2eDocsDir + "windows-utm" + lang.ext
		section := markdownSection(t, utm, readDoc(t, utm), lang.utmSelectionSection)
		lifecycle = append(lifecycle, scenarioList{utm + " " + lang.utmSelectionSection + " first command", firstRunPattern(t, utm, markdownCode(section))})
	}

	checkSameScenarioLists(t, "race and overload", race)
	checkSameScenarioLists(t, "event-loss notification", loss)
	checkSameScenarioLists(t, "Windows graceful lifecycle repeat", lifecycle)
	checkSameScenarioLists(t, "Windows notification process tree", notification)

	// harness.md 7단계에 따라, 경합·과부하 시나리오 가운데 Windows CI가 이름으로 따로 반복하지 않는 것은 -race 단계의 확인 항목에도 있어야 한다.
	// 이 문장은 Windows 전용 취소 시나리오도 담으므로 남는 이름은 허용하고, 두 판끼리는 같아야 한다.
	checkSameScenarioLists(t, "Windows CI -race step", raceStep)
	var notRepeated []string
	for _, name := range race[0].names {
		if !slices.Contains(lifecycle[0].names, name) {
			notRepeated = append(notRepeated, name)
		}
	}
	for _, list := range raceStep {
		if missing, _ := setDifference(list.names, notRepeated); len(missing) > 0 {
			t.Errorf("%s: missing race and overload scenarios %q that Windows CI does not repeat by name (from %s)", list.location, missing, race[0].location)
		}
	}
}
