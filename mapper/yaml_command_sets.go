package mapper

import (
	"github.com/seungyeop-lee/directory-watcher/runner"
)

type YamlCmd interface{}

type YamlPath string

type YamlPaths []YamlPath

type YamlExts []string

type YamlMillisecond uint

type YamlCommandSets struct {
	InitCmd   YamlCmd            `yaml:"initCmd"`
	EndCmd    YamlCmd            `yaml:"endCmd"`
	BeforeCmd YamlCmd            `yaml:"beforeCmd"`
	AfterCmd  YamlCmd            `yaml:"afterCmd"`
	Sets      YamlCommandSubSets `yaml:"sets"`
}

func (s YamlCommandSets) BuildCommandSets() runner.CommandSets {
	return runner.CommandSets{
		InitCmd:   NewYamlCmdConverter(s.InitCmd).Convert(),
		EndCmd:    NewYamlCmdConverter(s.EndCmd).Convert(),
		BeforeCmd: NewYamlCmdConverter(s.BeforeCmd).Convert(),
		AfterCmd:  NewYamlCmdConverter(s.AfterCmd).Convert(),
		Sets:      s.Sets.CommandSets(),
	}
}

type YamlCommandSubSets []YamlCommandSet

func (s YamlCommandSubSets) CommandSets() []runner.CommandSet {
	result := []runner.CommandSet{}
	for _, v := range s {
		result = append(result, runner.CommandSet{
			InitCmd:         NewYamlCmdConverter(v.InitCmd).Convert(),
			EndCmd:          NewYamlCmdConverter(v.EndCmd).Convert(),
			Cmd:             NewYamlCmdConverter(v.Cmd).Convert(),
			Path:            NewYamlPathConverter(v.Path).Convert(),
			ExcludeDir:      NewYamlPathsConverter(v.ExcludeDir).Convert(),
			ExcludeExt:      NewYamlExtsConverter(v.ExcludeExt).Convert(),
			WaitMillisecond: NewYamlWaitMillisecondConverter(v.WaitMillisecond).Convert(),
		})
	}
	return result
}

type YamlCommandSet struct {
	InitCmd         YamlCmd         `yaml:"initCmd"`
	EndCmd          YamlCmd         `yaml:"endCmd"`
	Cmd             YamlCmd         `yaml:"cmd"`
	Path            YamlPath        `yaml:"path"`
	ExcludeDir      YamlPaths       `yaml:"excludeDir"`
	ExcludeExt      YamlExts        `yaml:"excludeExt"`
	WaitMillisecond YamlMillisecond `yaml:"waitMillisecond"`
}
