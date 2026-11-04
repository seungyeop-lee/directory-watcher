package main

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	yamlconfig "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"gopkg.in/yaml.v3"
)

// 이 파일의 테스트는 스키마와 USAGE 두 판의 config.yml 블록에 손으로 적은 설정 키가 yaml 태그의 키와 같은지 확인한다.
// 키는 global.lifeCycle.onStartWatch처럼 위 키부터 점으로 이은 경로로 비교한다.
// 실패하면 directory-watcher.schema.json과 USAGE 두 판의 config.yml 블록을 yaml 태그에 맞춰 함께 고친다.

const schemaFile = "directory-watcher.schema.json"

// configKeys는 yamlconfig.Config의 yaml 태그에서 키 경로를 모으고, 하위 키를 가진 키의 경로를 parents에 표시한다.
// 목록 안의 키는 목록 키 바로 아래 키로 센다(watchTargets.path).
func configKeys() (keys []string, parents map[string]bool) {
	parents = map[string]bool{"": true}
	var walk func(t reflect.Type, parent string)
	walk = func(t reflect.Type, parent string) {
		for i := range t.NumField() {
			field := t.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			// yaml.Node 필드는 v3에서 없앤 키를 받아 오류로 알리려고 둔 것이라서, 스키마와 USAGE 예시에 없어야 한다.
			if name == "" || field.Type == reflect.TypeFor[yaml.Node]() {
				continue
			}
			key := joinKey(parent, name)
			keys = append(keys, key)
			value := field.Type
			if value.Kind() == reflect.Slice {
				value = value.Elem()
			}
			if value.Kind() == reflect.Struct {
				parents[key] = true
				walk(value, key)
			}
		}
	}
	walk(reflect.TypeFor[yamlconfig.Config](), "")
	return keys, parents
}

func joinKey(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
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

// schemaKeys는 스키마에서 parents에 든 키 아래의 속성 이름을 키 경로로 모은다.
// hook 명령의 dir, cmd처럼 yaml 태그가 하위 키를 정하지 않는 값의 속성은 모으지 않는다.
func schemaKeys(t *testing.T, parents map[string]bool) []string {
	t.Helper()
	root := readSchema(t)
	definitions, _ := root["definitions"].(map[string]any)
	var keys []string
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
			keys = append(keys, key)
			if parents[key] {
				walk(property.(map[string]any), key)
			}
		}
	}
	walk(root, "")
	return keys
}

var yamlKeyLine = regexp.MustCompile(`^( *)(- )?(\w+):`)

// usageConfigKeys는 USAGE의 config.yml 블록에서 parents에 든 키 아래의 키를 키 경로로 모은다.
func usageConfigKeys(t *testing.T, file string, parents map[string]bool) []string {
	t.Helper()
	var keys []string
	eachYAMLKey(usageConfigBlock(t, file), func(parent, key, _ string) {
		if parents[parent] {
			keys = append(keys, key)
		}
	})
	return keys
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

// checkSameKeys는 got이 want와 다르면, want에만 있는 키와 got에만 있는 키를 location과 함께 알린다.
func checkSameKeys(t *testing.T, location string, got, want []string) {
	t.Helper()
	var missing, unknown []string
	for _, key := range want {
		if !slices.Contains(got, key) {
			missing = append(missing, key)
		}
	}
	for _, key := range got {
		if !slices.Contains(want, key) {
			unknown = append(unknown, key)
		}
	}
	if len(missing) > 0 || len(unknown) > 0 {
		slices.Sort(missing)
		slices.Sort(unknown)
		t.Errorf("%s: missing keys %q, keys not in yaml tags %q", location, missing, unknown)
	}
}

func TestSchemaHasSameConfigKeysAsCode(t *testing.T) {
	want, parents := configKeys()
	checkSameKeys(t, schemaFile, schemaKeys(t, parents), want)
}

func TestUsageConfigBlockHasSameConfigKeysAsCode(t *testing.T) {
	want, parents := configKeys()
	for _, file := range usageFiles {
		checkSameKeys(t, file+" config.yml block", usageConfigKeys(t, file, parents), want)
	}
}
