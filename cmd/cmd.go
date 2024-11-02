package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/service"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/config"
	yamlConfig "github.com/seungyeop-lee/directory-watcher/v2/internal/config/yaml"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
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

	rootCmd.Flags().StringP("log-level", "l", helper.LogLevelStringDefaultValue, "set log level")
	rootCmd.Flags().StringP("config-path", "c", "config.yml", "set config path")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	RunE: func(cmd *cobra.Command, args []string) error {
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

		commandSet := config.BuildCommandSet()
		debugLogCommandSet(logger, commandSet)

		r := service.NewRunner(commandSet, logger)

		go r.Run()

		stopDone := make(chan bool)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			// 종료 시그널 발생 대기
			<-sigs
			// runner 정지
			r.Stop()
			// runner 정지 완료 신호 발생
			stopDone <- true
		}()
		// runner 정지 완료 신호 대기
		<-stopDone

		return nil
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
