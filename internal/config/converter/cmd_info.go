package converter

import (
	"reflect"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	cmd "github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain/cmdimpl"
)

type CmdInfo interface{}

type CmdInfoConverter struct {
	cmdInfo CmdInfo
}

func NewCmdInfoConverter(cmdInfo CmdInfo) *CmdInfoConverter {
	return &CmdInfoConverter{
		cmdInfo: cmdInfo,
	}
}

func (c CmdInfoConverter) Convert() domain.Cmd {
	v := reflect.ValueOf(c.cmdInfo)
	switch v.Kind() {
	case reflect.Slice:
		return c.mapForSlice(v)
	default:
		return cmd.EmptyCmd{}
	}
}

func (c CmdInfoConverter) mapForSlice(cmdInfoSliceValue reflect.Value) domain.Cmds {
	result := domain.Cmds{}
	cmdInfoSlice := cmdInfoSliceValue.Interface().([]interface{})
	for _, cmdInfo := range cmdInfoSlice {
		cmdInfoValue := reflect.ValueOf(cmdInfo)
		switch cmdInfoValue.Kind() {
		case reflect.String:
			result = append(result, c.mapToSingleCmd(cmdInfoValue))
		case reflect.Map:
			result = append(result, c.mapToStructuredCmd(cmdInfoValue))
		}
	}
	return result
}

func (c CmdInfoConverter) mapToSingleCmd(cmdStringValue reflect.Value) cmd.SingleCmd {
	s := cmdStringValue.Interface().(string)
	return cmd.SingleCmd(s)
}

func (c CmdInfoConverter) mapToStructuredCmd(cmdInfoMapValue reflect.Value) domain.Cmd {
	mapData := map[string]reflect.Value{}
	for _, k := range cmdInfoMapValue.MapKeys() {
		v := cmdInfoMapValue.MapIndex(k)
		if v.Kind() == reflect.Interface {
			v = v.Elem()
		}
		mapData[k.String()] = v
	}

	switch mapData["cmd"].Kind() {
	case reflect.String:
		if isEmptyDir(mapData["dir"]) {
			return c.mapToSingleCmd(mapData["cmd"])
		}
		return cmd.StructuredCmd{
			Cmd: c.mapToSingleCmd(mapData["cmd"]),
			Dir: domain.Path(mapData["dir"].Interface().(string)),
		}
	case reflect.Slice:
		if isEmptyDir(mapData["dir"]) {
			return c.mapForMultiLineCmd(mapData["cmd"])
		}
		return cmd.StructuredCmd{
			Cmd: c.mapForMultiLineCmd(mapData["cmd"]),
			Dir: domain.Path(mapData["dir"].Interface().(string)),
		}
	}

	return cmd.StructuredCmd{
		Cmd: cmd.EmptyCmd{},
		Dir: "",
	}
}

func isEmptyDir(dirValue reflect.Value) bool {
	switch dirValue.Kind() {
	case reflect.String:
		return false
	default:
		return true
	}
}

func (c CmdInfoConverter) mapForMultiLineCmd(cmdStringSliceValue reflect.Value) cmd.MultiLineCmd {
	result := cmd.MultiLineCmd{}
	cmdStringSlice := cmdStringSliceValue.Interface().([]interface{})
	for _, cmdString := range cmdStringSlice {
		cmdStringValue := reflect.ValueOf(cmdString)
		result.Cmds = append(result.Cmds, c.mapToSingleCmd(cmdStringValue))
	}
	return result
}
