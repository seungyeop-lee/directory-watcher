package converter

import (
	"fmt"
	"reflect"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

// CmdInfoConverter는 lifeCycle 아래 hook 하나의 명령을 만든다.
type CmdInfoConverter struct {
	key       string
	cmdInfo   config.CmdInfo
	shell     command.Shell
	eventHook bool
}

// NewCmdInfoConverter는 이벤트를 받지 않는 hook의 converter를 만든다. 이런 hook의 명령은 템플릿으로 해석하지 않고
// 그대로 실행하므로, Convert는 오류를 내지 않는다.
func NewCmdInfoConverter(key string, cmdInfo config.CmdInfo, shell command.Shell) *CmdInfoConverter {
	return &CmdInfoConverter{key: key, cmdInfo: cmdInfo, shell: shell}
}

// NewEventHookConverter는 이벤트를 받는 hook의 converter를 만든다. 명령이 모르는 템플릿 변수를 쓰면, Convert는
// hook 안에서 그 명령의 위치(예: onChange[1].cmd[0])를 앞에 붙인 오류를 낸다.
func NewEventHookConverter(key string, cmdInfo config.CmdInfo, shell command.Shell) *CmdInfoConverter {
	return &CmdInfoConverter{key: key, cmdInfo: cmdInfo, shell: shell, eventHook: true}
}

func (c CmdInfoConverter) Convert() (domain.Cmd, error) {
	builder := c.builder()
	if c.eventHook {
		if err := checkTemplates(builder); err != nil {
			return nil, fmt.Errorf("%s%w", c.key, err)
		}
	}
	return command.NewManager(builder), nil
}

func (c CmdInfoConverter) builder() command.ExecCmdBuilder {
	v := reflect.ValueOf(c.cmdInfo)
	switch v.Kind() {
	case reflect.Slice:
		return c.mapForSlice(v)
	default:
		return command.EmptyCmd{}
	}
}

// checkTemplates는 mapForSlice가 만든 명령을 설정 파일의 모양대로 따라가며 검사한다. hook 목록의 항목은 [i],
// 항목의 cmd 목록은 .cmd[j]로 위치를 적는다.
func checkTemplates(builder command.ExecCmdBuilder) error {
	switch b := builder.(type) {
	case command.Cmds:
		for i, item := range b {
			if err := checkTemplates(item); err != nil {
				return fmt.Errorf("[%d]%w", i, err)
			}
		}
	case command.StructuredCmd:
		return checkTemplates(b.Cmd)
	case command.MultiLineCmd:
		for i, line := range b.Cmds {
			if err := line.CheckTemplates(); err != nil {
				return fmt.Errorf(".cmd[%d]: %w", i, err)
			}
		}
	case command.SingleCmd:
		if err := b.CheckTemplates(); err != nil {
			return fmt.Errorf(": %w", err)
		}
	}
	return nil
}

func (c CmdInfoConverter) mapForSlice(cmdInfoSliceValue reflect.Value) command.Cmds {
	result := command.Cmds{}
	cmdInfoSlice := cmdInfoSliceValue.Interface().([]interface{})
	for _, cmdInfo := range cmdInfoSlice {
		cmdInfoValue := reflect.ValueOf(cmdInfo)
		switch cmdInfoValue.Kind() {
		case reflect.String:
			result = append(result, c.mapToSingleCmd(cmdInfoValue))
		case reflect.Map:
			result = append(result, c.mapForMap(cmdInfoValue))
		}
	}
	return result
}

func (c CmdInfoConverter) mapToSingleCmd(cmdStringValue reflect.Value) command.SingleCmd {
	s := cmdStringValue.Interface().(string)
	return command.SingleCmd{Shell: c.shell, Cmd: s}
}

func (c CmdInfoConverter) mapForMap(cmdInfoMapValue reflect.Value) command.ExecCmdBuilder {
	smi := newStructuredCmdInfo(cmdInfoMapValue)

	switch smi.cmdKind() {
	case reflect.String:
		if smi.dirIsEmpty() {
			return c.mapToSingleCmd(smi.cmdValue())
		}
		return command.StructuredCmd{
			Cmd: c.mapToSingleCmd(smi.cmdValue()),
			Dir: smi.dirToPath(),
		}
	case reflect.Slice:
		if smi.dirIsEmpty() {
			return c.mapToMultiLineCmd(smi.cmdValue())
		}
		return command.StructuredCmd{
			Cmd: c.mapToMultiLineCmd(smi.cmdValue()),
			Dir: smi.dirToPath(),
		}
	}

	return command.StructuredCmd{
		Cmd: command.EmptyCmd{},
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

func (c CmdInfoConverter) mapToMultiLineCmd(cmdStringSliceValue reflect.Value) command.MultiLineCmd {
	result := command.MultiLineCmd{}
	cmdStringSlice := cmdStringSliceValue.Interface().([]interface{})
	for _, cmdString := range cmdStringSlice {
		cmdStringValue := reflect.ValueOf(cmdString)
		result.Cmds = append(result.Cmds, c.mapToSingleCmd(cmdStringValue))
	}
	return result
}
