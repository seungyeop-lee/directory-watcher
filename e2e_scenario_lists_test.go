package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// 이 파일의 테스트는 e2e 문서에 손으로 적은 시나리오 목록과 그 목록을 가리키는 개수 낱말, 오류 주입 명령의 -run 대상, 시나리오 문서의 하위 테스트 목록이 e2e/scenarios의 시나리오와 맞는지 확인한다.
// 목록을 넣는 곳과 조건은 e2e/docs/harness.md "Writing a new scenario"의 5–7단계가 정한다.
// 실패하면 보고된 목록의 시나리오 이름이나 하위 테스트 이름, 문구의 개수를 고치고, 한국어판과 영어판을 함께 고친다.

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
	// subtestsSection은 시나리오 문서의 하위 테스트 절 제목이고, subtestColumns는 하위 테스트 이름을 적는 표의 첫 칸 머리말이다.
	subtestsSection string
	subtestColumns  []string
	// 아래 문구는 windows 문서가 Windows CI의 정상 종료 반복 목록과 알림 트리 검증 목록을 이름 없이 개수로 가리키는 곳이다.
	// 첫 묶음이 개수 낱말이다.
	ciStepsLifecycleTests        *regexp.Regexp
	ciResultLifecycleTests       *regexp.Regexp
	ciResultLifecycleSubtests    *regexp.Regexp
	ciResultNotificationSubtests *regexp.Regexp
}

var e2eDocLanguages = []e2eDocLanguage{
	{
		ext:                          ".md",
		readmeRow:                    "Per-scenario documents: ",
		raceSection:                  "## Race and overload verification",
		lossSection:                  "## Event-loss notification verification",
		windowsRunSection:            "## Run commands",
		windowsCIStepsSection:        "## CI steps",
		windowsCIResult:              "## Checking CI results",
		utmSelectionSection:          "### Test selection, repetition, and time limits",
		subtestsSection:              "## Subtests",
		subtestColumns:               []string{"Subtest", "Mode"},
		ciStepsLifecycleTests:        regexp.MustCompile(`repeat of the (\w+) tests in \[Run commands\]`),
		ciResultLifecycleTests:       regexp.MustCompile(`the (\w+) tests and \w+ option combinations in the repeat step`),
		ciResultLifecycleSubtests:    regexp.MustCompile(`the \w+ tests and (\w+) option combinations in the repeat step`),
		ciResultNotificationSubtests: regexp.MustCompile(`the (\w+) modes in the notification verification step`),
	},
	{
		ext:                          ".ko.md",
		readmeRow:                    "시나리오별 문서: ",
		raceSection:                  "## 경합과 과부하 검증",
		lossSection:                  "## 이벤트 유실 알림 검증",
		windowsRunSection:            "## 실행 명령",
		windowsCIStepsSection:        "## CI 단계",
		windowsCIResult:              "## CI 결과 확인",
		utmSelectionSection:          "### 테스트 선택·반복·시간 제한",
		subtestsSection:              "## 하위 테스트",
		subtestColumns:               []string{"하위 테스트", "모드"},
		ciStepsLifecycleTests:        regexp.MustCompile(`\(#실행-명령\)에 나온 (\S+) 테스트`),
		ciResultLifecycleTests:       regexp.MustCompile(`반복 단계의 (\S+) 테스트와 \S+ 옵션 조합`),
		ciResultLifecycleSubtests:    regexp.MustCompile(`반복 단계의 \S+ 테스트와 (\S+) 옵션 조합`),
		ciResultNotificationSubtests: regexp.MustCompile(`알림 검증 단계의 (\S+) 모드`),
	},
}

// scenarioList는 문서나 ci.yml의 한 곳에 적힌 시나리오 이름 목록이다.
type scenarioList struct {
	location string
	names    []string
}

var scenarioFunc = regexp.MustCompile(`(?m)^func (Test\w+)\(\w+ \*testing\.T\)`)

// e2eScenarioSources는 e2e/scenarios/*_test.go의 최상위 Test 함수마다 이름과 소스 텍스트를 모은다.
// 함수의 소스는 선언 줄부터 첫 칸에 있는 } 줄 앞까지다.
// windows_cancel_test.go처럼 빌드 태그가 붙은 파일의 시나리오도 OS와 관계없이 모으려고 컴파일하지 않고 읽는다.
func e2eScenarioSources(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(e2eScenariosDir, "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, file := range files {
		text := readDoc(t, file)
		for _, m := range scenarioFunc.FindAllStringSubmatchIndex(text, -1) {
			source, _, _ := strings.Cut(text[m[0]:], "\n}\n")
			sources[text[m[2]:m[3]]] = source
		}
	}
	if len(sources) == 0 {
		t.Fatalf("no scenario test functions in %s", e2eScenariosDir)
	}
	return sources
}

// e2eScenarioNames는 e2e/scenarios의 시나리오 이름이다.
func e2eScenarioNames(t *testing.T) []string {
	t.Helper()
	return slices.Sorted(maps.Keys(e2eScenarioSources(t)))
}

// scenarioDocName은 harness.md 5단계의 규칙대로 시나리오 이름에서 Test를 떼고 kebab-case로 바꾼 문서 이름이다.
// 대문자가 이어지는 약어는 한 단어로 본다(TestQueueWarningCLI → queue-warning-cli).
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
		// CI 단계 절은 정상 종료 반복 시나리오를 이름 없이 개수로만 가리키므로, 이 절의 시나리오 이름은 알림 트리 검증 시나리오다.
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

// countWords는 문서가 개수를 적는 영어 낱말과 한국어 수 관형사를 수로 바꾼다.
var countWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
	"한": 1, "두": 2, "세": 3, "네": 4, "다섯": 5, "여섯": 6,
	"일곱": 7, "여덟": 8, "아홉": 9, "열": 10, "열한": 11, "열두": 12,
}

// checkCountWord는 doc에서 phrase에 맞는 문구마다 첫 묶음의 개수 낱말이 want와 같은지 확인한다.
// 문구가 없거나, 낱말을 수로 읽지 못하거나, 수가 다르면 file의 줄 번호와 문구를 알린다. counted에는 want가 무엇을 센 값인지 적는다.
func checkCountWord(t *testing.T, file, doc string, phrase *regexp.Regexp, want int, counted string) {
	t.Helper()
	matches := phrase.FindAllStringSubmatchIndex(doc, -1)
	if len(matches) == 0 {
		t.Errorf("%s: no phrase matching `%s`", file, phrase)
	}
	for _, m := range matches {
		line := strings.Count(doc[:m[0]], "\n") + 1
		word := doc[m[2]:m[3]]
		if got, known := countWords[word]; !known {
			t.Errorf("%s:%d: cannot read %q in %q as a count word", file, line, word, doc[m[0]:m[1]])
		} else if got != want {
			t.Errorf("%s:%d: %q says %d, want %d (%s)", file, line, doc[m[0]:m[1]], got, want, counted)
		}
	}
}

// scenarioSubtests는 시나리오 하나가 t.Run으로 만드는 하위 테스트의 이름이다. 이름을 소스에서 읽지 못하면 err에 그 까닭이 있다.
type scenarioSubtests struct {
	names []string
	err   error
}

// e2eScenarioSubtests는 e2eScenarioSources의 시나리오마다 하위 테스트 이름을 소스에서 읽는다.
// e2eScenarioSources처럼 빌드 태그가 붙은 파일도 OS와 관계없이 읽으려고, 컴파일하지 않고 구문 트리만 만든다.
func e2eScenarioSubtests(t *testing.T) map[string]scenarioSubtests {
	t.Helper()
	scenarios := e2eScenarioSources(t)
	files, err := filepath.Glob(filepath.Join(e2eScenariosDir, "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	subtests := map[string]scenarioSubtests{}
	for _, file := range files {
		syntax, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range syntax.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv != nil {
				continue
			}
			if _, isScenario := scenarios[fn.Name.Name]; !isScenario {
				continue
			}
			r := subtestReader{fset: fset, locals: map[string]ast.Expr{}}
			r.walk(fn.Body, map[string]any{}, false)
			subtests[fn.Name.Name] = scenarioSubtests{r.names, r.err}
		}
	}
	return subtests
}

// subtestReader는 시나리오 함수에서 t.Run의 첫 인자를 실행하지 않고 계산한다.
// 이름은 문자열·정수·bool 리터럴, 리터럴 slice를 도는 range 변수와 그 struct 필드, 문자열 +, fmt.Sprintf로 만든 것만 읽는다.
// range가 도는 slice는 리터럴이거나, 리터럴을 := 로 받은 지역 변수여야 한다.
type subtestReader struct {
	fset *token.FileSet
	// locals는 := 로 선언한 지역 변수의 오른쪽 식이다.
	locals map[string]ast.Expr
	names  []string
	err    error
}

// walk는 node 안의 t.Run마다 이름을 계산해 names에 더한다. env는 둘러싼 range 변수의 값이다.
// inUnknownLoop이면 t.Run이 몇 번 도는지 알 수 없으므로, t.Run을 만나면 err에 알린다.
func (r *subtestReader) walk(node ast.Node, env map[string]any, inUnknownLoop bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		if r.err != nil {
			return false
		}
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE && len(n.Lhs) == len(n.Rhs) {
				for i, lhs := range n.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						r.locals[id.Name] = n.Rhs[i]
					}
				}
			}
		case *ast.ForStmt:
			r.walk(n.Body, env, true)
			return false
		case *ast.RangeStmt:
			r.walkRange(n, env, inUnknownLoop)
			return false
		case *ast.CallExpr:
			if !isCall(n, "t", "Run") {
				break
			}
			position := r.fset.Position(n.Pos())
			if inUnknownLoop {
				r.err = fmt.Errorf("%s: cannot read subtest names: t.Run is inside a loop whose values are not literals", position)
				return false
			}
			name, ok := r.eval(n.Args[0], env)
			if s, isString := name.(string); ok && isString {
				r.names = append(r.names, s)
			} else {
				r.err = fmt.Errorf("%s: cannot read the subtest name from the first argument of t.Run", position)
			}
			// 하위 테스트의 본문은 읽지 않는다.
			return false
		}
		return true
	})
}

// walkRange는 loop가 리터럴 slice를 돌면 요소마다 range 변수를 그 값으로 두고 본문을 읽는다.
// 리터럴이 아닌 것을 돌면 본문을 몇 번 도는지 모르는 반복으로 읽는다.
func (r *subtestReader) walkRange(loop *ast.RangeStmt, env map[string]any, inUnknownLoop bool) {
	x := loop.X
	if id, ok := x.(*ast.Ident); ok {
		x = r.locals[id.Name]
	}
	list, isLiteral := x.(*ast.CompositeLit)
	if !isLiteral {
		r.walk(loop.Body, env, true)
		return
	}
	value, _ := loop.Value.(*ast.Ident)
	for _, elt := range list.Elts {
		inner := maps.Clone(env)
		if value != nil {
			// 읽지 못한 요소의 range 변수가 바깥 range 변수의 값을 쓰지 않게 지운다.
			delete(inner, value.Name)
			if v, ok := r.eval(elt, env); ok {
				inner[value.Name] = v
			}
		}
		r.walk(loop.Body, inner, inUnknownLoop)
	}
}

// eval은 expr의 값을 string, int, bool, 또는 struct 리터럴의 필드 이름과 값을 담은 map으로 계산한다.
// struct 리터럴에서 계산하지 못한 필드는 빼고, 그 밖의 식을 계산하지 못하면 ok가 false다.
func (r *subtestReader) eval(expr ast.Expr, env map[string]any) (value any, ok bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return r.eval(e.X, env)
	case *ast.BasicLit:
		var err error
		switch e.Kind {
		case token.STRING:
			value, err = strconv.Unquote(e.Value)
			return value, err == nil
		case token.INT:
			value, err = strconv.Atoi(e.Value)
			return value, err == nil
		}
	case *ast.Ident:
		if v, found := env[e.Name]; found {
			return v, true
		}
		switch e.Name {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	case *ast.SelectorExpr:
		x, _ := r.eval(e.X, env)
		if fields, isStruct := x.(map[string]any); isStruct {
			v, found := fields[e.Sel.Name]
			return v, found
		}
	case *ast.BinaryExpr:
		x, _ := r.eval(e.X, env)
		y, _ := r.eval(e.Y, env)
		xs, isString := x.(string)
		ys, alsoString := y.(string)
		if e.Op == token.ADD && isString && alsoString {
			return xs + ys, true
		}
	case *ast.CallExpr:
		if !isCall(e, "fmt", "Sprintf") || len(e.Args) == 0 {
			break
		}
		var args []any
		for _, arg := range e.Args {
			v, ok := r.eval(arg, env)
			if !ok {
				return nil, false
			}
			args = append(args, v)
		}
		if format, isString := args[0].(string); isString {
			return fmt.Sprintf(format, args[1:]...), true
		}
	case *ast.CompositeLit:
		fields := map[string]any{}
		for _, elt := range e.Elts {
			kv, isField := elt.(*ast.KeyValueExpr)
			if !isField {
				return nil, false
			}
			key, isName := kv.Key.(*ast.Ident)
			if !isName {
				return nil, false
			}
			if v, ok := r.eval(kv.Value, env); ok {
				fields[key.Name] = v
			}
		}
		return fields, true
	}
	return nil, false
}

// isCall은 call이 x.fn(...) 꼴이고 x가 이름 name인 패키지나 변수인지 본다.
func isCall(call *ast.CallExpr, name, fn string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != fn {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == name
}

// windows 문서는 Windows CI의 정상 종료 반복 목록과 알림 트리 검증 목록, 그 하위 테스트를 이름 없이 개수로도 가리킨다.
// 이 개수를 ci.yml 단계의 -run 목록과 시나리오 소스에서 센 수와 비교한다. 목록끼리 같은지는 TestE2EDocsSubsetListsAgree가 본다.
func TestE2EDocsCountWordsMatchLists(t *testing.T) {
	scenarios := e2eScenarioSubtests(t)
	workflow := readDoc(t, ciWorkflowFile)
	lifecycleStep := "Repeat Windows graceful lifecycle without test cache"
	notificationStep := "Verify Windows error notification process trees"
	lifecycle := firstRunPattern(t, ciWorkflowFile, ciStep(t, workflow, lifecycleStep))
	notification := firstRunPattern(t, ciWorkflowFile, ciStep(t, workflow, notificationStep))
	subtests := func(names []string) int {
		total := 0
		for _, name := range names {
			subtests, found := scenarios[name]
			if !found {
				t.Errorf("%s: %s is not a scenario in %s", ciWorkflowFile, name, e2eScenariosDir)
			} else if subtests.err != nil {
				t.Errorf("%s: %v", name, subtests.err)
			}
			total += len(subtests.names)
		}
		return total
	}
	lifecycleSubtests, notificationSubtests := subtests(lifecycle), subtests(notification)

	for _, lang := range e2eDocLanguages {
		windows := e2eDocsDir + "windows" + lang.ext
		doc := readDoc(t, windows)
		checkCountWord(t, windows, doc, lang.ciStepsLifecycleTests, len(lifecycle), fmt.Sprintf("scenarios %q of the %s step", lifecycle, lifecycleStep))
		checkCountWord(t, windows, doc, lang.ciResultLifecycleTests, len(lifecycle), fmt.Sprintf("scenarios %q of the %s step", lifecycle, lifecycleStep))
		checkCountWord(t, windows, doc, lang.ciResultLifecycleSubtests, lifecycleSubtests, fmt.Sprintf("subtests of %q", lifecycle))
		checkCountWord(t, windows, doc, lang.ciResultNotificationSubtests, notificationSubtests, fmt.Sprintf("subtests of %q", notification))
	}
}

// unreadableSubtests는 하위 테스트 이름을 소스에서 읽을 수 없어 문서와 비교하지 않는 시나리오와 그 까닭이다.
var unreadableSubtests = map[string]string{
	"TestCanceledPowershellHookStops": "t.Run runs in a for loop over time.Duration, and the scenario documents give the name pattern after=<time> instead of each name",
}

var subtestBullet = regexp.MustCompile("^- `([^`]+)`")

// docSubtestLists는 시나리오 문서 doc에서 하위 테스트 이름을 적은 목록을 모두 모은다.
// 첫 칸 머리말이 lang.subtestColumns 가운데 하나인 표가 한 목록이고, 그 표의 첫 칸에는 백틱으로 감싼 이름 하나만 둔다.
// 하위 테스트 절에 그런 표가 없으면, 그 절에서 백틱으로 감싼 이름으로 시작하는 bullet이 한 목록이다.
func docSubtestLists(t *testing.T, file, doc string, lang e2eDocLanguage) []scenarioList {
	t.Helper()
	isSubtestTable := func(table markdownTable) bool { return slices.Contains(lang.subtestColumns, table.header[0]) }
	var lists []scenarioList
	for _, table := range markdownTables(doc) {
		if !isSubtestTable(table) {
			continue
		}
		list := scenarioList{location: fmt.Sprintf("%s %q table", file, table.header[0])}
		for _, cells := range table.rows {
			name, found := strings.CutPrefix(cells[0], "`")
			name, quoted := strings.CutSuffix(name, "`")
			if !found || !quoted || strings.Contains(name, "`") {
				t.Errorf("%s: cannot read a subtest name from the first cell %q", list.location, cells[0])
				continue
			}
			list.names = append(list.names, name)
		}
		lists = append(lists, list)
	}
	if !strings.Contains(doc, "\n"+lang.subtestsSection+"\n") {
		return lists
	}
	section := markdownSection(t, file, doc, lang.subtestsSection)
	if slices.ContainsFunc(markdownTables(section), isSubtestTable) {
		return lists
	}
	bullets := scenarioList{location: file + " " + lang.subtestsSection + " bullets"}
	for _, line := range strings.Split(section, "\n") {
		if m := subtestBullet.FindStringSubmatch(line); m != nil {
			bullets.names = append(bullets.names, m[1])
		}
	}
	return append(lists, bullets)
}

// 시나리오 문서 두 판의 하위 테스트 목록마다, 적힌 이름이 시나리오 소스의 t.Run 이름과 같은지 확인한다.
func TestE2EDocsSubtestListsMatchCode(t *testing.T) {
	for name, subtests := range e2eScenarioSubtests(t) {
		if reason, excluded := unreadableSubtests[name]; excluded {
			if subtests.err == nil {
				t.Errorf("%s: subtest names are now readable, so remove it from unreadableSubtests (%s)", name, reason)
			}
			continue
		}
		if subtests.err != nil {
			t.Errorf("%s: %v; make the names readable or add it to unreadableSubtests with the reason", name, subtests.err)
			continue
		}
		for _, lang := range e2eDocLanguages {
			file := e2eDocsDir + "scenarios/" + scenarioDocName(name) + lang.ext
			lists := docSubtestLists(t, file, readDoc(t, file), lang)
			if len(lists) == 0 && len(subtests.names) > 0 {
				t.Errorf("%s: no subtest list for the subtests %q of %s", file, subtests.names, name)
			}
			for _, list := range lists {
				if missing, extra := setDifference(list.names, subtests.names); len(missing) > 0 || len(extra) > 0 {
					t.Errorf("%s: missing subtests %q, names not in the t.Run calls of %s %q", list.location, missing, name, extra)
				}
			}
		}
	}
}

// faultRunPatterns는 text에서 DW_E2E_FAULT를 설정한 채 실행하는 명령 줄의 -run 정규식을 줄마다 모은다.
// 설정은 명령 앞에 붙인 DW_E2E_FAULT=<값>이거나, PowerShell에서 $env:DW_E2E_FAULT = <값> 줄부터 Remove-Item Env:DW_E2E_FAULT 줄 앞까지다.
func faultRunPatterns(t *testing.T, file, text string) []scenarioList {
	t.Helper()
	var commands []scenarioList
	faultSet := false
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "$env:DW_E2E_FAULT =") {
			faultSet = true
		} else if strings.Contains(line, "Remove-Item Env:DW_E2E_FAULT") {
			faultSet = false
		}
		if !faultSet && !strings.Contains(line, "DW_E2E_FAULT=") {
			continue
		}
		location := fmt.Sprintf("%s:%d", file, i+1)
		for _, names := range runPatterns(t, location, line) {
			commands = append(commands, scenarioList{location, names})
		}
	}
	return commands
}

// 오류 주입 명령은 DW_E2E_FAULT를 읽는 시나리오만 -run으로 골라야 의도한 실패와 시간 초과를 일으킨다.
func TestE2EDocsFaultCommandsRunFaultScenarios(t *testing.T) {
	var want []string
	for name, source := range e2eScenarioSources(t) {
		if strings.Contains(source, `"DW_E2E_FAULT"`) {
			want = append(want, name)
		}
	}
	files := []string{ciWorkflowFile}
	for _, lang := range e2eDocLanguages {
		for _, doc := range []string{"diagnostics", "windows", "windows-utm"} {
			files = append(files, e2eDocsDir+doc+lang.ext)
		}
	}
	for _, file := range files {
		commands := faultRunPatterns(t, file, readDoc(t, file))
		if len(commands) == 0 {
			t.Errorf("%s: no -run command that runs with DW_E2E_FAULT set", file)
		}
		for _, command := range commands {
			if missing, extra := setDifference(command.names, want); len(missing) > 0 || len(extra) > 0 {
				t.Errorf("%s: -run of the DW_E2E_FAULT command misses %q and has %q not reading DW_E2E_FAULT", command.location, missing, extra)
			}
		}
	}
}
