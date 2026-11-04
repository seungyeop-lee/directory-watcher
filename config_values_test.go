package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	command "github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/cmd"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config/converter"
)

// 이 파일의 테스트는 정해진 값만 받는 옵션에서, 스키마의 enum·pattern과 USAGE 두 판, config.example.yml에 손으로 적은
// 값 목록이 코드가 받는 값 목록과 같은지 확인한다.
// 실패하면 directory-watcher.schema.json, USAGE 두 판의 config.yml 블록·표·오류 예시, config.example.yml의 주석에서
// 그 옵션의 값 목록을 코드 목록에 맞춰 함께 고친다. 스키마에 enum이나 pattern을 새로 적었으면 fixedValueOptions에도 더한다.

// fixedValueOption은 정해진 값만 받는 옵션이다.
type fixedValueOption struct {
	key    string   // 위 키부터 점으로 이은 키 경로
	values []string // 코드가 받는 값
	// letters이면 values의 글자를 하나 이상 이어 적거나 none을 적는다(fileEvent: CU). 아니면 values에서 하나를 적는다.
	letters bool
}

func fixedValueOptions() []fixedValueOption {
	return []fixedValueOption{
		{key: "shell", values: command.ShellNames()},
		{key: "watchTargets.option.watch.fileEvent", values: strings.Split(converter.FileEventKinds, ""), letters: true},
		{key: "watchTargets.option.watch.dirEvent", values: strings.Split(converter.DirEventKinds, ""), letters: true},
		{key: "watchTargets.option.hookBatch.deliver", values: converter.DeliverNames()},
		{key: "watchTargets.option.run.interrupt", values: converter.InterruptNames()},
	}
}

// name은 USAGE의 표 머리 행처럼 키 경로의 마지막 키만 쓰는 곳에서 옵션을 부르는 이름이다.
func (o fixedValueOption) name() string {
	return o.key[strings.LastIndex(o.key, ".")+1:]
}

// schemaProperties는 스키마의 속성을 모두 키 경로별로 모은다. 목록 안의 키는 목록 키 바로 아래 키로 센다(watchTargets.path).
func schemaProperties(t *testing.T) map[string]map[string]any {
	t.Helper()
	root := readSchema(t)
	definitions, _ := root["definitions"].(map[string]any)
	found := map[string]map[string]any{}
	var walk func(node map[string]any, parent string)
	walk = func(node map[string]any, parent string) {
		if ref, ok := node["$ref"].(string); ok {
			node, _ = definitions[strings.TrimPrefix(ref, "#/definitions/")].(map[string]any)
		}
		if items, ok := node["items"].(map[string]any); ok {
			node = items
		}
		properties, _ := node["properties"].(map[string]any)
		for name, property := range properties {
			key := joinKey(parent, name)
			found[key] = property.(map[string]any)
			walk(found[key], key)
		}
	}
	walk(root, "")
	return found
}

func TestSchemaFixedValuesMatchCode(t *testing.T) {
	properties := schemaProperties(t)
	var restricted, covered []string
	for key, property := range properties {
		_, hasEnum := property["enum"]
		_, hasPattern := property["pattern"]
		if hasEnum || hasPattern {
			restricted = append(restricted, key)
		}
	}
	for _, option := range fixedValueOptions() {
		covered = append(covered, option.key)
	}
	if !sameSet(restricted, covered) {
		slices.Sort(restricted)
		t.Errorf("%s: keys with an enum or a pattern %q, keys this test compares %q", schemaFile, restricted, covered)
	}

	for _, option := range fixedValueOptions() {
		property := properties[option.key]
		if !option.letters {
			var got []string
			enum, _ := property["enum"].([]any)
			for _, value := range enum {
				got = append(got, value.(string))
			}
			if !sameSet(got, option.values) {
				t.Errorf("%s: %s enum = %q, want %q", schemaFile, option.key, got, option.values)
			}
			continue
		}
		// pattern은 정규식이라서 목록을 뽑지 않고, 대문자 하나씩 받는지 물어 받는 글자를 모은다.
		source, _ := property["pattern"].(string)
		pattern, err := regexp.Compile(source)
		if err != nil {
			t.Errorf("%s: %s pattern: %v", schemaFile, option.key, err)
			continue
		}
		var got []string
		for letter := 'A'; letter <= 'Z'; letter++ {
			if pattern.MatchString(string(letter)) {
				got = append(got, string(letter))
			}
		}
		if !sameSet(got, option.values) {
			t.Errorf("%s: %s pattern %q accepts the letters %q, want %q", schemaFile, option.key, source, got, option.values)
		}
		for _, value := range []string{noneValue, strings.Join(option.values, "")} {
			if !pattern.MatchString(value) {
				t.Errorf("%s: %s pattern %q rejects %q", schemaFile, option.key, source, value)
			}
		}
	}
}

// noneValue는 letters 옵션에서 이벤트를 하나도 받지 않음을 뜻하는 값이다.
const noneValue = "none"

var (
	// usageChoiceList는 USAGE config.yml 블록의 주석이 값을 "(last, lastPerPath, each)"처럼 괄호 안에 적은 목록이다.
	usageChoiceList = regexp.MustCompile(`\(([\w, ]+)\)`)
	// letterLabel은 글자마다 뜻을 붙인 "C: create or move in"에서 글자다.
	letterLabel = regexp.MustCompile(`\b([A-Z]): `)
	// tableValueCell은 USAGE 표의 첫 칸 "`last` (default)"에서 값이다.
	tableValueCell = regexp.MustCompile("^`(\\w+)`")
)

func TestUsageListsSameFixedValuesAsCode(t *testing.T) {
	for _, file := range usageFiles {
		lines := map[string]string{}
		eachYAMLKey(usageConfigBlock(t, file), func(_, key, line string) { lines[key] = line })
		tables := markdownTables(readDoc(t, file))
		for _, option := range fixedValueOptions() {
			line, ok := lines[option.key]
			if !ok {
				t.Errorf("%s config.yml block has no %s", file, option.key)
				continue
			}
			var got []string
			if option.letters {
				for _, m := range letterLabel.FindAllStringSubmatch(line, -1) {
					got = append(got, m[1])
				}
			} else if m := usageChoiceList.FindStringSubmatch(line); m != nil {
				got = strings.Split(m[1], ", ")
			}
			if !sameSet(got, option.values) {
				t.Errorf("%s config.yml block: %s lists %q, want %q", file, option.key, got, option.values)
			}

			// 글자를 이어 적는 옵션은 config.yml 블록에서만 글자를 안내하고 표를 두지 않는다.
			if option.letters {
				continue
			}
			got = nil
			for _, table := range tables {
				if table.header[0] != "`"+option.name()+"`" {
					continue
				}
				for _, cells := range table.rows {
					if m := tableValueCell.FindStringSubmatch(cells[0]); m != nil {
						got = append(got, m[1])
					}
				}
			}
			if !sameSet(got, option.values) {
				t.Errorf("%s `%s` table = %q, want %q", file, option.name(), got, option.values)
			}
		}
	}
}

// exampleChoiceList는 config.example.yml의 주석이 기본값 뒤에 값을 "; last, lastPerPath, or each"처럼 적은 목록이다.
var exampleChoiceList = regexp.MustCompile(`# default \S+; ((?:\w+, )*\w+,? or \w+)$`)

func TestExampleConfigListsSameFixedValuesAsCode(t *testing.T) {
	values := map[string][]string{}
	for _, option := range fixedValueOptions() {
		values[option.key] = option.values
	}
	compared := 0
	// config.example.yml은 예시에 쓴 키에만 주석을 달기 때문에, 값 목록을 적은 줄만 비교한다.
	eachYAMLKey(readDoc(t, exampleConfigFile), func(_, key, line string) {
		m := exampleChoiceList.FindStringSubmatch(line)
		if m == nil {
			return
		}
		want, ok := values[key]
		if !ok {
			t.Errorf("%s: %s lists values, but it is not an option with fixed values", exampleConfigFile, key)
			return
		}
		compared++
		got := regexp.MustCompile(`,? (?:or )?`).Split(m[1], -1)
		if !sameSet(got, want) {
			t.Errorf("%s: %s lists %q, want %q", exampleConfigFile, key, got, want)
		}
	})
	if compared == 0 {
		t.Errorf("%s: found no list of values", exampleConfigFile)
	}
}

// USAGE 두 판의 오류 예시는 아래 값을 적었을 때의 실제 오류 메시지를 옮겨 적은 것이라서, 값 목록이 바뀌면 메시지도 바뀐다.
func TestUsageErrorExamplesListSameValuesAsCode(t *testing.T) {
	_, shellErr := command.LookupShell("bash")
	interrupt := "true"
	_, interruptErr := converter.NewInterruptConverter(&interrupt).Convert()
	dirEvent := config.WatchEvent("U")
	_, dirEventErr := converter.NewDirEventConverter(&dirEvent).Convert()
	examples := []struct {
		key string
		err error
	}{
		{"shell", shellErr},
		{"run.interrupt", interruptErr},
		{"watch.dirEvent", dirEventErr},
	}
	for _, example := range examples {
		if example.err == nil {
			t.Fatalf("%s: the code accepted the value of the error example", example.key)
		}
		for _, file := range usageFiles {
			if !strings.Contains(readDoc(t, file), example.err.Error()) {
				t.Errorf("%s has no error example %q", file, example.err.Error())
			}
		}
	}
}
