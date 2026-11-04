package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	yamlconfig "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"gopkg.in/yaml.v3"
)

// 이 파일의 테스트는 스키마의 watchTargets.option 아래에 손으로 적은 default가 코드의 기본값과 같은지 확인한다.
// 키 하나에 스키마의 default를 적은 설정이 아무 키도 적지 않은 설정과 같은 옵션을 만들면 두 값이 같다고 본다.
// 실패하면 directory-watcher.schema.json의 default를 코드의 기본값에 맞추고, 같은 기본값을 적은 USAGE 두 판과
// config.example.yml의 주석도 함께 고친다.

// schemaOptionDefaults는 스키마의 watchTargets.option 아래에서 default가 있는 속성을 hookBatch.deliver처럼
// option 아래 키 경로와 default 값으로 모은다.
func schemaOptionDefaults(t *testing.T) map[string]any {
	t.Helper()
	option := schemaProperty(readSchema(t), "watchTargets")
	option = schemaProperty(option["items"].(map[string]any), "option")
	defaults := map[string]any{}
	var walk func(node map[string]any, parent string)
	walk = func(node map[string]any, parent string) {
		properties, _ := node["properties"].(map[string]any)
		for name, property := range properties {
			property := property.(map[string]any)
			key := joinKey(parent, name)
			if value, ok := property["default"]; ok {
				defaults[key] = value
			}
			walk(property, key)
		}
	}
	walk(option, "")
	return defaults
}

func schemaProperty(node map[string]any, name string) map[string]any {
	properties, _ := node["properties"].(map[string]any)
	property, _ := properties[name].(map[string]any)
	return property
}

// optionKeysWithCodeDefault는 yamlconfig.WatchTargetsOptionConfig에서 포인터로 받는 키를 option 아래 키 경로로 모은다.
// 포인터로 받는 키는 적지 않은 것과 적은 것을 구별해 적지 않으면 기본값을 쓰므로, 스키마에도 default가 있어야 한다.
func optionKeysWithCodeDefault() []string {
	var keys []string
	for group := range reflect.TypeFor[yamlconfig.WatchTargetsOptionConfig]().Fields() {
		if group.Type.Kind() != reflect.Struct || group.Type == reflect.TypeFor[yaml.Node]() {
			continue
		}
		for field := range group.Type.Fields() {
			if field.Type.Kind() == reflect.Pointer {
				keys = append(keys, joinKey(yamlKey(group), yamlKey(field)))
			}
		}
	}
	return keys
}

func yamlKey(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
	return name
}

// optionWith는 option 아래 키 경로 key 하나에 value를 적은 설정으로 옵션을 만든다.
// YAML은 JSON을 그대로 읽으므로, 스키마에서 읽은 값을 JSON으로 적어 yaml 태그로 읽는다.
func optionWith(t *testing.T, key string, value any) yamlconfig.WatchTargetsOptionConfig {
	t.Helper()
	names := strings.Split(key, ".")
	for _, name := range slices.Backward(names) {
		value = map[string]any{name: value}
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var option yamlconfig.WatchTargetsOptionConfig
	if err := yaml.Unmarshal(data, &option); err != nil {
		t.Fatalf("option.%s: %v", key, err)
	}
	return option
}

func TestSchemaOptionDefaultsMatchCodeDefaults(t *testing.T) {
	defaults := schemaOptionDefaults(t)
	var keys []string
	for key := range defaults {
		keys = append(keys, key)
	}
	want := optionKeysWithCodeDefault()
	if !sameSet(keys, want) {
		slices.Sort(keys)
		slices.Sort(want)
		t.Errorf("%s: option keys with a default %q, option keys the code fills in when omitted %q", schemaFile, keys, want)
	}

	codeDefaults, err := yamlconfig.WatchTargetsOptionConfig{}.BuildOption()
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range defaults {
		got, err := optionWith(t, key, value).BuildOption()
		if err != nil {
			t.Errorf("%s: default %v of option.%s is rejected: %v", schemaFile, value, key, err)
			continue
		}
		if !reflect.DeepEqual(got, codeDefaults) {
			t.Errorf("%s: default %v of option.%s differs from the code default\nwith the default: %+v\nwith nothing set: %+v", schemaFile, value, key, got, codeDefaults)
		}
	}
}
