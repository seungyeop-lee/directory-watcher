package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/service"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
	yamlConfig "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/spf13/cobra"
)

var (
	executeFileName string
	version         string
)

func Execute() {
	executeFileName = filepath.Base(os.Args[0])
	rootCmd.Use = executeFileName

	if version == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			version = info.Main.Version
		}
	}
	rootCmd.Version = version

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("log-level", "l", helper.LogLevelStringDefaultValue, "set log level")
	rootCmd.Flags().StringP("config-path", "c", "config.yml", "set config path")
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

		configFile, err := getConfigFile(cmd)
		if err != nil {
			return err
		}

		config := yamlConfig.NewConfig(configFile)
		debugLogConfig(logger, config)

		commandSet, err := config.BuildCommandSet()
		if err != nil {
			return err
		}
		debugLogCommandSet(logger, commandSet)

		r := service.NewRunner(commandSet, logger)

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

func getConfigFile(cmd *cobra.Command) ([]byte, error) {
	configPath, err := cmd.Flags().GetString("config-path")
	if err != nil {
		return nil, err
	}

	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	return configFile, nil
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
