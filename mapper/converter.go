package mapper

import (
	"reflect"

	"github.com/seungyeop-lee/directory-watcher/runner"
)

type YamlCmdConverter struct {
	yamlCmd YamlCmd
}

func NewYamlCmdConverter(yamlCmd YamlCmd) *YamlCmdConverter {
	return &YamlCmdConverter{
		yamlCmd: yamlCmd,
	}
}

func (c YamlCmdConverter) Convert() runner.Cmd {
	v := reflect.ValueOf(c.yamlCmd)
	switch v.Kind() {
	case reflect.String:
		s := v.Interface().(string)
		return runner.SingleCmd(s)
	case reflect.Slice:
		result := runner.MultiLineCmd{}
		cmdInterfaces := v.Interface().([]interface{})
		for _, cmdInterface := range cmdInterfaces {
			v2 := reflect.ValueOf(cmdInterface)
			result.Cmds = append(result.Cmds, runner.SingleCmd(v2.Interface().(string)))
		}
		return result
	default:
		return runner.EmptyCmd{}
	}
}

type YamlPathConverter struct {
	yamlPath YamlPath
}

func NewYamlPathConverter(yamlPath YamlPath) *YamlPathConverter {
	return &YamlPathConverter{yamlPath: yamlPath}
}

func (c YamlPathConverter) Convert() runner.Path {
	return runner.Path(c.yamlPath)
}

type YamlPathsConverter struct {
	yamlPaths YamlPaths
}

func NewYamlPathsConverter(yamlPaths YamlPaths) *YamlPathsConverter {
	return &YamlPathsConverter{yamlPaths: yamlPaths}
}

func (c YamlPathsConverter) Convert() runner.Paths {
	result := runner.Paths{}
	for _, p := range c.yamlPaths {
		result = append(result, runner.Path(p))
	}
	return result
}

type YamlWaitMillisecondConverter struct {
	yamlMillisecond YamlMillisecond
}

func NewYamlWaitMillisecondConverter(yamlMillisecond YamlMillisecond) *YamlWaitMillisecondConverter {
	return &YamlWaitMillisecondConverter{yamlMillisecond: yamlMillisecond}
}

func (c YamlWaitMillisecondConverter) Convert() runner.Millisecond {
	return runner.Millisecond(c.yamlMillisecond)
}
