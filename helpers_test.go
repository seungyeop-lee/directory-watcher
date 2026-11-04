package main

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var usageFiles = []string{"USAGE.md", "USAGE.ko.md"}

const (
	schemaFile        = "directory-watcher.schema.json"
	exampleConfigFile = "config.example.yml"
)

// readDoc은 저장소 루트의 문서 file을 읽는다.
// Windows의 git은 줄 끝을 CRLF로 바꿔 받을 수 있으므로, 줄 단위로 맞추는 검사가 OS와 관계없이 같도록 줄 끝을 LF로 바꾼다.
func readDoc(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// usageConfigBlock은 USAGE의 "## config.yml" 절에 있는 첫 yaml 블록을 돌려준다.
func usageConfigBlock(t *testing.T, file string) string {
	t.Helper()
	_, section, found := strings.Cut(readDoc(t, file), "\n## config.yml\n")
	_, block, _ := strings.Cut(section, "```yaml\n")
	block, _, closed := strings.Cut(block, "\n```")
	if !found || !closed {
		t.Fatalf("%s has no yaml block in the config.yml section", file)
	}
	return block
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

func readSchema(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return root
}

// eachSchemaProperty는 스키마의 속성을 위에서부터 돌며, 속성마다 위 키의 경로, 키 경로, 속성 노드를 visit에 넘긴다.
// 키 경로는 eachYAMLField와 같이 위 키부터 점으로 잇고, 목록 안의 키는 목록 키 바로 아래 키로 센다(watchTargets.path).
// $ref, items, properties만 따라가고 anyOf, oneOf, allOf, additionalProperties 안의 속성은 넘기지 않는다.
func eachSchemaProperty(t *testing.T, visit func(parent, key string, property map[string]any)) {
	t.Helper()
	root := readSchema(t)
	definitions, _ := root["definitions"].(map[string]any)
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
			property := property.(map[string]any)
			key := joinKey(parent, name)
			visit(parent, key, property)
			walk(property, key)
		}
	}
	walk(root, "")
}

var yamlKeyLine = regexp.MustCompile(`^( *)(- )?(\w+):`)

// eachYAMLKey는 yaml 문서에서 키를 적은 줄마다 위 키의 경로, 키 경로, 줄을 visit에 넘긴다.
// 키의 위 키는 줄의 들여쓰기로 정하고, "- "로 시작하는 목록 항목의 키는 "- " 뒤의 열에 있는 것으로 본다.
func eachYAMLKey(doc string, visit func(parent, key, line string)) {
	type openKey struct {
		column int
		path   string
	}
	var open []openKey
	for _, line := range strings.Split(doc, "\n") {
		m := yamlKeyLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		column := len(m[1]) + len(m[2])
		for len(open) > 0 && open[len(open)-1].column >= column {
			open = open[:len(open)-1]
		}
		parent := ""
		if len(open) > 0 {
			parent = open[len(open)-1].path
		}
		key := joinKey(parent, m[3])
		visit(parent, key, line)
		open = append(open, openKey{column, key})
	}
}

// eachYAMLField는 struct 포인터 root 안의 yaml 태그 필드를 위에서부터 돌며, 필드마다 키 경로와 필드 값을 visit에 넘긴다.
// 키 경로는 global.lifeCycle.onStartWatch처럼 위 키부터 점으로 잇고, 목록 안의 키는 목록 키 바로 아래 키로 센다(watchTargets.path).
// struct 필드와 목록의 struct 항목 안으로 내려가되, v3에서 없앤 키를 받는 yaml.Node 필드 안으로는 내려가지 않는다.
// 목록에 항목이 없으면 빈 항목 하나를 만들어 돌기 때문에 빈 설정에서도 모든 키를 넘긴다. 그 항목의 필드를 고쳐도 root는 바뀌지 않는다.
func eachYAMLField(root any, visit func(key string, field reflect.Value)) {
	var walk func(v reflect.Value, parent string)
	walk = func(v reflect.Value, parent string) {
		for i := range v.NumField() {
			name := yamlKey(v.Type().Field(i))
			if name == "" {
				continue
			}
			key, field := joinKey(parent, name), v.Field(i)
			visit(key, field)
			switch {
			case field.Type() == reflect.TypeFor[yaml.Node]():
			case field.Kind() == reflect.Struct:
				walk(field, key)
			case field.Kind() == reflect.Slice:
				items := field
				if items.Len() == 0 {
					items = reflect.MakeSlice(field.Type(), 1, 1)
				}
				for j := range items.Len() {
					if items.Index(j).Kind() == reflect.Struct {
						walk(items.Index(j), key)
					}
				}
			}
		}
	}
	walk(reflect.ValueOf(root).Elem(), "")
}

func yamlKey(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
	return name
}

func joinKey(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

func sameSet(got, want []string) bool {
	got, want = slices.Clone(got), slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}
