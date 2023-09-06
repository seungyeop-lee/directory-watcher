package converter

import (
	"reflect"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain/cmd"
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
		return mapForSlice(v)
	default:
		return cmd.EmptyCmd{}
	}
}

func mapForSlice(cmdInfoSliceValue reflect.Value) cmd.Cmds {
	result := cmd.Cmds{}
	cmdInfoSlice := cmdInfoSliceValue.Interface().([]interface{})
	for _, cmdInfo := range cmdInfoSlice {
		cmdInfoValue := reflect.ValueOf(cmdInfo)
		switch cmdInfoValue.Kind() {
		case reflect.String:
			result = append(result, mapToSingleCmd(cmdInfoValue))
		case reflect.Map:
			result = append(result, mapForMap(cmdInfoValue))
		}
	}
	return result
}

func mapToSingleCmd(cmdStringValue reflect.Value) cmd.SingleCmd {
	s := cmdStringValue.Interface().(string)
	return cmd.SingleCmd(s)
}

func mapForMap(cmdInfoMapValue reflect.Value) domain.Cmd {
	smi := newStructuredCmdInfo(cmdInfoMapValue)

	switch smi.cmdKind() {
	case reflect.String:
		if smi.dirIsEmpty() {
			return mapToSingleCmd(smi.cmdValue())
		}
		return cmd.StructuredCmd{
			Cmd: mapToSingleCmd(smi.cmdValue()),
			Dir: smi.dirToPath(),
		}
	case reflect.Slice:
		if smi.dirIsEmpty() {
			return mapToMultiLineCmd(smi.cmdValue())
		}
		return cmd.StructuredCmd{
			Cmd: mapToMultiLineCmd(smi.cmdValue()),
			Dir: smi.dirToPath(),
		}
	}

	return cmd.StructuredCmd{
		Cmd: cmd.EmptyCmd{},
		Dir: "",
	}
}

type structuredCmdInfo map[string]reflect.Value

func newStructuredCmdInfo(cmdInfoMapValue reflect.Value) structuredCmdInfo {
	mapData := map[string]reflect.Value{}
	for _, k := range cmdInfoMapValue.MapKeys() {
		v := cmdInfoMapValue.MapIndex(k)
		if v.Kind() == reflect.Interface {
			v = v.Elem()
		}
		mapData[k.String()] = v
	}

	return mapData
}

func (s structuredCmdInfo) cmdKind() reflect.Kind {
	return s.cmdValue().Kind()
}

func (s structuredCmdInfo) dirToPath() domain.Path {
	return domain.Path(s.dirValue().Interface().(string))
}

func (s structuredCmdInfo) dirIsEmpty() bool {
	switch s.dirValue().Kind() {
	case reflect.String:
		return false
	default:
		return true
	}
}

func (s structuredCmdInfo) cmdValue() reflect.Value {
	return s["cmd"]
}

func (s structuredCmdInfo) dirValue() reflect.Value {
	return s["dir"]
}

func mapToMultiLineCmd(cmdStringSliceValue reflect.Value) cmd.MultiLineCmd {
	result := cmd.MultiLineCmd{}
	cmdStringSlice := cmdStringSliceValue.Interface().([]interface{})
	for _, cmdString := range cmdStringSlice {
		cmdStringValue := reflect.ValueOf(cmdString)
		result.Cmds = append(result.Cmds, mapToSingleCmd(cmdStringValue))
	}
	return result
}
