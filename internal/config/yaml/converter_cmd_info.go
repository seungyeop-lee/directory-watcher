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
	case reflect.Slice:
		return c.mapForSlice(v)
	default:
		return domain.EmptyCmd{}
	}
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
		}
	}
	return result
}

func (c CmdInfoConverter) mapToSingleCmd(v reflect.Value) domain.SingleCmd {
	s := v.Interface().(string)
	return domain.SingleCmd(s)
}

func (c CmdInfoConverter) mapToStructuredCmd(m reflect.Value) domain.Cmd {
	mapData := map[string]reflect.Value{}
	for _, k := range m.MapKeys() {
		v := m.MapIndex(k)
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
		return domain.StructuredCmd{
			Cmd: c.mapToSingleCmd(mapData["cmd"]),
			Dir: domain.Path(mapData["dir"].Interface().(string)),
		}
	case reflect.Slice:
		if isEmptyDir(mapData["dir"]) {
			return c.mapForMultiLineCmd(mapData["cmd"])
		}
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

func isEmptyDir(dirValue reflect.Value) bool {
	switch dirValue.Kind() {
	case reflect.String:
		return false
	default:
		return true
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
