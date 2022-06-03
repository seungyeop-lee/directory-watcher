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
		return c.mapToSingleCmd(v)
	case reflect.Map:
		return c.mapToStructuredCmd(v)
	case reflect.Slice:
		return c.mapForSlice(v)
	default:
		return runner.EmptyCmd{}
	}
}

func (c YamlCmdConverter) mapToSingleCmd(v reflect.Value) runner.SingleCmd {
	s := v.Interface().(string)
	return runner.SingleCmd(s)
}

func (c YamlCmdConverter) mapToStructuredCmd(v reflect.Value) runner.StructuredCmd {
	mapData := map[string]reflect.Value{}
	for _, e := range v.MapKeys() {
		index := v.MapIndex(e)
		if index.Kind() == reflect.Interface {
			index = index.Elem()
		}
		mapData[e.String()] = index
	}

	switch mapData["cmd"].Kind() {
	case reflect.String:
		return runner.StructuredCmd{
			Cmd: c.mapToSingleCmd(mapData["cmd"]),
			Dir: runner.Path(mapData["dir"].Interface().(string)),
		}
	case reflect.Slice:
		return runner.StructuredCmd{
			Cmd: c.mapForMultiLineCmd(mapData["cmd"]),
			Dir: runner.Path(mapData["dir"].Interface().(string)),
		}
	}

	return runner.StructuredCmd{
		Cmd: runner.EmptyCmd{},
		Dir: "",
	}
}

func (c YamlCmdConverter) mapForMultiLineCmd(v reflect.Value) runner.MultiLineCmd {
	result := runner.MultiLineCmd{}
	cmdInterfaces := v.Interface().([]interface{})
	for _, cmdInterface := range cmdInterfaces {
		v2 := reflect.ValueOf(cmdInterface)
		result.Cmds = append(result.Cmds, c.mapToSingleCmd(v2))
	}
	return result
}

func (c YamlCmdConverter) mapForSlice(v reflect.Value) runner.Cmds {
	result := runner.Cmds{}
	cmdInterfaces := v.Interface().([]interface{})
	for _, cmdInterface := range cmdInterfaces {
		v2 := reflect.ValueOf(cmdInterface)
		switch v2.Kind() {
		case reflect.String:
			result = append(result, c.mapToSingleCmd(v2))
		case reflect.Map:
			result = append(result, c.mapToStructuredCmd(v2))
		case reflect.Slice:
			result = append(result, c.mapForMultiLineCmd(v2))
		}
	}
	return result
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

type YamlPathSuffixesConverter struct {
	yamlPathSuffixes YamlPathSuffixes
}

func NewYmlPathSuffixesConverter(yamlPathSuffixes YamlPathSuffixes) *YamlPathSuffixesConverter {
	return &YamlPathSuffixesConverter{yamlPathSuffixes: yamlPathSuffixes}
}

func (c YamlPathSuffixesConverter) Convert() runner.PathSuffixes {
	result := runner.PathSuffixes{}
	for _, e := range c.yamlPathSuffixes {
		result = append(result, runner.PathSuffix(e))
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
