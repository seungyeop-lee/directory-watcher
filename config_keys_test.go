package main

import (
	"reflect"
	"slices"
	"testing"

	yamlconfig "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"gopkg.in/yaml.v3"
)

// 이 파일의 테스트는 스키마와 USAGE 두 판의 config.yml 블록에 손으로 적은 설정 키가 yaml 태그의 키와 같은지 확인한다.
// 키는 global.lifeCycle.onStartWatch처럼 위 키부터 점으로 이은 경로로 비교한다.
// 실패하면 directory-watcher.schema.json과 USAGE 두 판의 config.yml 블록을 yaml 태그에 맞춰 함께 고친다.

// configKeys는 yamlconfig.Config의 yaml 태그에서 키 경로를 모으고, 하위 키를 가진 키의 경로를 parents에 표시한다.
func configKeys() (keys []string, parents map[string]bool) {
	parents = map[string]bool{"": true}
	eachYAMLField(&yamlconfig.Config{}, func(key string, field reflect.Value) {
		// yaml.Node 필드는 v3에서 없앤 키를 받아 오류로 알리려고 둔 것이라서, 스키마와 USAGE 예시에 없어야 한다.
		if field.Type() == reflect.TypeFor[yaml.Node]() {
			return
		}
		keys = append(keys, key)
		value := field.Type()
		if value.Kind() == reflect.Slice {
			value = value.Elem()
		}
		if value.Kind() == reflect.Struct {
			parents[key] = true
		}
	})
	return keys, parents
}

// schemaKeys는 스키마에서 parents에 든 키 아래의 속성 이름을 키 경로로 모은다.
// hook 명령의 dir, cmd는 스키마의 anyOf 안에 있어서 eachSchemaProperty가 넘기지 않으므로 모이지 않는다.
func schemaKeys(t *testing.T, parents map[string]bool) []string {
	t.Helper()
	var keys []string
	eachSchemaProperty(t, func(parent, key string, _ map[string]any) {
		if parents[parent] {
			keys = append(keys, key)
		}
	})
	return keys
}

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
