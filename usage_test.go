package main

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	command "github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/cmd"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/service"
	yamlconfig "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"gopkg.in/yaml.v3"
)

// 이 파일의 테스트는 USAGE 두 판에 손으로 옮겨 적은 목록이 코드가 정의하는 목록과 같은지 확인한다.
// 실패하면 USAGE.md와 USAGE.ko.md의 해당 표와 예시를 코드 목록에 맞춰 함께 고친다.
// 없앤 키의 오류가 인용하는 USAGE 절 제목은 반대로 코드(internal/config/yaml/config.go)에 손으로 적혀 있으므로, 그 검사가 실패하면 코드의 제목을 USAGE.md의 절 제목에 맞춘다.

var usageFiles = []string{"USAGE.md", "USAGE.ko.md"}

func readUsage(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// markdownTable은 문서의 표 하나다. 칸의 앞뒤 공백은 뺀다.
type markdownTable struct {
	header []string
	rows   [][]string
}

// markdownTables는 문서에서 구분 행(|---|) 바로 위의 행을 머리 행으로 하는 표를 모두 찾는다.
func markdownTables(doc string) []markdownTable {
	var tables []markdownTable
	lines := strings.Split(doc, "\n")
	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "|-") {
			continue
		}
		table := markdownTable{header: tableCells(lines[i-1])}
		for i++; i < len(lines) && strings.HasPrefix(lines[i], "|"); i++ {
			table.rows = append(table.rows, tableCells(lines[i]))
		}
		tables = append(tables, table)
	}
	return tables
}

func tableCells(line string) []string {
	cells := strings.Split(strings.Trim(line, "|"), "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

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

func sameSet(got, want []string) bool {
	got, want = slices.Clone(got), slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
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
		doc := readUsage(t, file)
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

func TestUsageListsSameErrorVariablesAsCode(t *testing.T) {
	want := service.ErrorVariableNames()
	for _, file := range usageFiles {
		got, _ := matchingRows(readUsage(t, file), regexp.MustCompile("^`(DW_ERROR_\\w+)`$"))
		if !sameSet(got, want) {
			t.Errorf("%s DW_ERROR_* table = %q, want %q", file, got, want)
		}
	}
}

func TestUsageListsSameQueueWarningVariablesAsCode(t *testing.T) {
	want := service.QueueWarningVariableNames()
	for _, file := range usageFiles {
		got, _ := matchingRows(readUsage(t, file), regexp.MustCompile("^`(DW_QUEUE_\\w+)`$"))
		if !sameSet(got, want) {
			t.Errorf("%s DW_QUEUE_* table = %q, want %q", file, got, want)
		}
	}
}

func TestUsageListsSameShellsAsCode(t *testing.T) {
	want := command.ShellNames()
	_, err := command.LookupShell("bash")
	if err == nil {
		t.Fatal("LookupShell accepted an unknown shell")
	}
	for _, file := range usageFiles {
		doc := readUsage(t, file)
		var got []string
		for _, table := range markdownTables(doc) {
			if table.header[0] != "`shell`" {
				continue
			}
			for _, cells := range table.rows {
				got = append(got, strings.Trim(cells[0], "`"))
			}
		}
		if !sameSet(got, want) {
			t.Errorf("%s shell table = %q, want %q", file, got, want)
		}
		if list := "(" + strings.Join(want, ", ") + ")"; !strings.Contains(doc, list) {
			t.Errorf("%s config.yml comment has no shell list %q", file, list)
		}
		if !strings.Contains(doc, err.Error()) {
			t.Errorf("%s has no error example %q", file, err.Error())
		}
	}
}

// eachRemovedKeyNode는 c 안에 있는 yaml.Node 필드마다 키 경로와 필드를 visit에 넘긴다.
// yaml.Node 필드는 v3에서 없앤 키를 받아 오류로 알리려고 둔 것이다. 목록 안의 키는 목록 키 바로 아래 키로 센다(watchTargets.option.excludeDir).
func eachRemovedKeyNode(c *yamlconfig.Config, visit func(key string, node *yaml.Node)) {
	var walk func(v reflect.Value, parent string)
	walk = func(v reflect.Value, parent string) {
		for i := range v.NumField() {
			name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("yaml"), ",")
			if name == "" {
				continue
			}
			key, field := joinKey(parent, name), v.Field(i)
			if node, ok := field.Addr().Interface().(*yaml.Node); ok {
				visit(key, node)
				continue
			}
			switch field.Kind() {
			case reflect.Struct:
				walk(field, key)
			case reflect.Slice:
				for j := range field.Len() {
					if field.Index(j).Kind() == reflect.Struct {
						walk(field.Index(j), key)
					}
				}
			}
		}
	}
	walk(reflect.ValueOf(c).Elem(), "")
}

func TestRemovedKeyErrorsCiteExistingUsageSections(t *testing.T) {
	headings := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^#+ (.+)$`).FindAllStringSubmatch(readUsage(t, "USAGE.md"), -1) {
		headings[m[1]] = true
	}
	parse := func() yamlconfig.Config {
		var c yamlconfig.Config
		if err := yaml.Unmarshal([]byte("watchTargets:\n  - path: site\n"), &c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	base := parse()
	var keys []string
	eachRemovedKeyNode(&base, func(key string, _ *yaml.Node) { keys = append(keys, key) })
	if len(keys) == 0 {
		t.Fatal("found no removed key fields")
	}
	citation := regexp.MustCompile(`see USAGE "([^"]+)"`)
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			c := parse()
			eachRemovedKeyNode(&c, func(k string, node *yaml.Node) {
				if k == key {
					node.Kind = yaml.ScalarNode
				}
			})
			_, err := c.BuildCommandSet()
			if err == nil {
				t.Fatal("BuildCommandSet accepted the removed key")
			}
			m := citation.FindStringSubmatch(err.Error())
			if m == nil {
				t.Fatalf("error %q cites no USAGE section", err)
			}
			if !headings[m[1]] {
				t.Errorf("error %q cites USAGE section %q, which USAGE.md has no heading for", err, m[1])
			}
		})
	}
}
