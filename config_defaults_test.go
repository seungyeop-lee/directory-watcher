package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	yamlconfig "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"gopkg.in/yaml.v3"
)

// 이 파일의 테스트는 스키마의 watchTargets.option 아래에 손으로 적은 default가 코드의 기본값과 같은지,
// USAGE 두 판의 config.yml 블록과 config.example.yml의 주석에 적은 기본값이 스키마의 default와 같은지 확인한다.
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
	eachYAMLField(&yamlconfig.WatchTargetsOptionConfig{}, func(key string, field reflect.Value) {
		if field.Kind() == reflect.Pointer {
			keys = append(keys, key)
		}
	})
	return keys
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

// documentedOptionDefaults는 doc의 option 아래 키 줄에서 phrase가 뽑은 기본값을 option 아래 키 경로별로 모은다.
// config.example.yml처럼 감시 대상이 여럿이면 같은 키의 기본값이 여러 개일 수 있다.
func documentedOptionDefaults(doc string, phrase *regexp.Regexp) map[string][]string {
	defaults := map[string][]string{}
	eachYAMLKey(doc, func(_, key, line string) {
		key, found := strings.CutPrefix(key, "watchTargets.option.")
		if m := phrase.FindStringSubmatch(line); found && m != nil {
			defaults[key] = append(defaults[key], m[1])
		}
	})
	return defaults
}

func TestDocumentedOptionDefaultsMatchSchema(t *testing.T) {
	// 기본값 문구는 값으로 시작하고, maxWaitMillisecond처럼 규칙으로 정하는 기본값은 값 뒤에 규칙이 붙으므로
	// 문구의 첫 낱말만 스키마의 default와 비교한다. 정규식의 \w는 ASCII만 받아서 "1000이고"에서도 1000을 뽑는다.
	docs := []struct {
		location string
		text     string
		phrase   *regexp.Regexp
		// all이면 스키마에 default가 있는 키마다 기본값 문구가 있어야 한다.
		// config.example.yml은 예시에 쓴 키에만 주석을 달기 때문에 all이 아니다.
		all bool
	}{
		{"USAGE.md config.yml block", usageConfigBlock(t, "USAGE.md"), regexp.MustCompile(`default is "?(\w+)`), true},
		{"USAGE.ko.md config.yml block", usageConfigBlock(t, "USAGE.ko.md"), regexp.MustCompile(`default는 "?(\w+)`), true},
		{exampleConfigFile + " comments", readDoc(t, exampleConfigFile), regexp.MustCompile(`# default (\w+)`), false},
	}
	schemaDefaults := schemaOptionDefaults(t)
	for _, doc := range docs {
		documented := documentedOptionDefaults(doc.text, doc.phrase)
		if len(documented) == 0 {
			t.Errorf("%s: found no option default", doc.location)
		}
		for key, value := range schemaDefaults {
			if _, ok := documented[key]; !ok && doc.all {
				t.Errorf("%s: option.%s has no default, schema default is %v", doc.location, key, value)
			}
		}
		for key, values := range documented {
			value, ok := schemaDefaults[key]
			if !ok {
				t.Errorf("%s: option.%s has default %q, which the schema does not have", doc.location, key, values)
				continue
			}
			for _, got := range values {
				if want := fmt.Sprint(value); got != want {
					t.Errorf("%s: option.%s default is %s, schema default is %s", doc.location, key, got, want)
				}
			}
		}
	}
}
