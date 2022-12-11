package yaml

import (
	"reflect"

	"github.com/seungyeop-lee/directory-watcher/internal/app/domain"
)

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
	case reflect.String:
		return c.mapToSingleCmd(v)
	case reflect.Map:
		return c.mapToStructuredCmd(v)
	case reflect.Slice:
		return c.mapForSlice(v)
	default:
		return domain.EmptyCmd{}
	}
}

func (c CmdInfoConverter) mapToSingleCmd(v reflect.Value) domain.SingleCmd {
	s := v.Interface().(string)
	return domain.SingleCmd(s)
}

func (c CmdInfoConverter) mapToStructuredCmd(v reflect.Value) domain.StructuredCmd {
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
		return domain.StructuredCmd{
			Cmd: c.mapToSingleCmd(mapData["cmd"]),
			Dir: domain.Path(mapData["dir"].Interface().(string)),
		}
	case reflect.Slice:
		return domain.StructuredCmd{
			Cmd: c.mapForMultiLineCmd(mapData["cmd"]),
			Dir: domain.Path(mapData["dir"].Interface().(string)),
		}
	}

	return domain.StructuredCmd{
		Cmd: domain.EmptyCmd{},
		Dir: "",
	}
}

func (c CmdInfoConverter) mapForMultiLineCmd(v reflect.Value) domain.MultiLineCmd {
	result := domain.MultiLineCmd{}
	cmdInterfaces := v.Interface().([]interface{})
	for _, cmdInterface := range cmdInterfaces {
		v2 := reflect.ValueOf(cmdInterface)
		result.Cmds = append(result.Cmds, c.mapToSingleCmd(v2))
	}
	return result
}

func (c CmdInfoConverter) mapForSlice(v reflect.Value) domain.Cmds {
	result := domain.Cmds{}
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
