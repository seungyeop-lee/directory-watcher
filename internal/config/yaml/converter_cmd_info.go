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

func (c CmdInfoConverter) mapToStructuredCmd(m reflect.Value) domain.StructuredCmd {
	mapData := map[string]reflect.Value{}
	for _, k := range m.MapKeys() {
		v := m.MapIndex(k)
		if v.Kind() == reflect.Interface {
			v = v.Elem()
		}
		mapData[k.String()] = v
	}

	dir := mapData["dir"].Interface().(string)

	switch mapData["cmd"].Kind() {
	case reflect.String:
		return domain.StructuredCmd{
			Cmd: c.mapToSingleCmd(mapData["cmd"]),
			Dir: domain.Path(dir),
		}
	case reflect.Slice:
		return domain.StructuredCmd{
			Cmd: c.mapForMultiLineCmd(mapData["cmd"]),
			Dir: domain.Path(dir),
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

func (c CmdInfoConverter) mapForSlice(s reflect.Value) domain.Cmds {
	result := domain.Cmds{}
	cmdInterfaces := s.Interface().([]interface{})
	for _, cmdInterface := range cmdInterfaces {
		v := reflect.ValueOf(cmdInterface)
		switch v.Kind() {
		case reflect.String:
			result = append(result, c.mapToSingleCmd(v))
		case reflect.Map:
			result = append(result, c.mapToStructuredCmd(v))
		case reflect.Slice:
			result = append(result, c.mapForMultiLineCmd(v))
		}
	}
	return result
}
