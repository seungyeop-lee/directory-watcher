package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/service"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
	yamlConfig "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	executeFileName string
	version         string
)

// defaultConfigPaths는 -c가 없을 때 작업 폴더에서 차례로 찾는 설정 파일 이름이다.
var defaultConfigPaths = []string{"config.directory-watcher.yml", "config.yml"}

func Execute() {
	executeFileName = filepath.Base(os.Args[0])
	rootCmd.Use = executeFileName

	if version == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			version = info.Main.Version
		}
	}
	rootCmd.Version = version

	restoreConsole := useUTF8ConsoleOutput()
	err := rootCmd.Execute()
	// os.Exit은 defer를 실행하지 않으므로, 종료 코드를 정하기 전에 되돌린다.
	restoreConsole()
	if err != nil {
		os.Exit(1)
	}
}

// maintenanceFlags는 개발과 유지보수에 쓰는 flag다. usage는 이 flag를 일반 flag와 다른 절에 보여 준다.
var maintenanceFlags = pflag.NewFlagSet("maintenance", pflag.ContinueOnError)

func init() {
	maintenanceFlags.StringP("log-level", "l", helper.LogLevelStringDefaultValue, "set log level")
	rootCmd.Flags().AddFlagSet(maintenanceFlags)
	rootCmd.Flags().StringP("config-path", "c", "", "set config path (default: "+strings.Join(defaultConfigPaths, ", then ")+")")
	rootCmd.Flags().BoolP("silent", "s", false, "don't print the config file path or hook commands")
	rootCmd.Flags().BoolP("dry-run", "n", false, "print hook commands without running them")
	rootCmd.MarkFlagsMutuallyExclusive("silent", "dry-run")
	rootCmd.SetUsageFunc(printUsage)
}

// printUsage는 cobra의 기본 usage와 같은 형식으로 쓰되, maintenanceFlags를 Maintenance Flags 절로 떼어 낸다.
// --help와 잘못된 flag가 모두 이 함수로 usage를 출력한다. rootCmd에는 하위 명령, 별칭, 예시가 없어서 기본 usage의
// 그 절들은 옮기지 않았다.
func printUsage(cmd *cobra.Command) error {
	general := pflag.NewFlagSet("general", pflag.ContinueOnError)
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if maintenanceFlags.Lookup(f.Name) == nil {
			general.AddFlag(f)
		}
	})
	_, err := fmt.Fprintf(cmd.OutOrStderr(), "Usage:\n  %s\n\nFlags:\n%s\n\nMaintenance Flags:\n%s\n",
		cmd.UseLine(),
		strings.TrimRight(general.FlagUsages(), "\n"),
		strings.TrimRight(maintenanceFlags.FlagUsages(), "\n"))
	return err
}

var rootCmd = &cobra.Command{
	RunE: func(cmd *cobra.Command, args []string) error {
		// 플래그 해석이 끝난 뒤에 켜야 잘못된 플래그에는 사용법이 그대로 나오고,
		// 실행 중 오류에는 Error: 줄만 나온다.
		cmd.SilenceUsage = true

		logger, err := getLogger(cmd)
		if err != nil {
			return err
		}
		helper.GlobalLogger = logger

		mode, err := getCommandMode(cmd)
		if err != nil {
			return err
		}

		configPath, configFile, err := getConfigFile(cmd)
		if err != nil {
			return err
		}
		// 설정 오류 메시지에는 파일 이름이 없으므로, 설정을 검사하기 전에 어느 파일을 읽었는지 알린다.
		if !mode.Silent {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "config: %s\n", configPath)
		}

		config := yamlConfig.NewConfig(configFile)
		debugLogConfig(logger, config)

		commandSet, err := config.BuildCommandSet()
		if err != nil {
			return err
		}
		debugLogCommandSet(logger, commandSet)

		r := service.NewRunner(commandSet, logger, mode)

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigs)
		go r.Run()
		var fatal error
		select {
		case fatal = <-r.Errors():
		case <-sigs:
		}
		r.Stop()
		// 종료하는 동안 이미 받은 오류를 성공으로 끝내면 안 된다.
		if fatal == nil {
			select {
			case fatal = <-r.Errors():
			default:
			}
		}
		return fatal
	},
}

func getLogger(cmd *cobra.Command) (helper.Logger, error) {
	logLevelStr, err := cmd.Flags().GetString("log-level")
	if err != nil {
		return nil, err
	}

	logLevel := helper.LogLevelString(logLevelStr).GetLogLevel()
	logger := helper.NewBasicLogger(logLevel)

	return logger, nil
}

func getCommandMode(cmd *cobra.Command) (process.CommandMode, error) {
	silent, err := cmd.Flags().GetBool("silent")
	if err != nil {
		return process.CommandMode{}, err
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return process.CommandMode{}, err
	}
	return process.CommandMode{Silent: silent, DryRun: dryRun}, nil
}

// getConfigFile은 -c로 받은 경로를, -c가 없으면 defaultConfigPaths 가운데 처음 있는 파일을 읽고, 읽은 경로와 내용을 돌려준다.
func getConfigFile(cmd *cobra.Command) (path string, data []byte, err error) {
	path, err = cmd.Flags().GetString("config-path")
	if err != nil {
		return "", nil, err
	}
	if path != "" {
		data, err = os.ReadFile(path)
		return path, data, err
	}

	for _, candidate := range defaultConfigPaths {
		data, err = os.ReadFile(candidate)
		if !errors.Is(err, fs.ErrNotExist) {
			return candidate, data, err
		}
	}
	return "", nil, fmt.Errorf("no config file found: tried %s (use -c to set a path)", strings.Join(defaultConfigPaths, ", "))
}

func debugLogConfig(logger helper.Logger, config config.Config) {
	logger.Debug("--- yaml config file map result ---")
	configJsonStr, _ := json.MarshalIndent(config, "", "	")
	logger.Debug(bytes.NewBuffer(configJsonStr).String())
}

func debugLogCommandSet(logger helper.Logger, commandSet domain.CommandSet) {
	logger.Debug("--- command set struct ---")
	commandSetJsonStr, _ := json.MarshalIndent(commandSet, "", "	")
	logger.Debug(bytes.NewBuffer(commandSetJsonStr).String())
}
