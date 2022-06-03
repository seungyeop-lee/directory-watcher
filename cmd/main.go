package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/seungyeop-lee/directory-watcher/helper"
	"github.com/seungyeop-lee/directory-watcher/mapper"
	"github.com/seungyeop-lee/directory-watcher/runner"
)

var (
	cfgPath   string
	isVerbose bool
	isDebug   bool
)

func main() {
	fmt.Println("directory-watcher run")

	defer func() {
		if e := recover(); e != nil {
			log.Fatalf("PANIC: %+v", e)
		}
	}()

	flag.StringVar(&cfgPath, "c", "", "config path")
	flag.BoolVar(&isVerbose, "v", false, "verbose")
	flag.BoolVar(&isDebug, "d", false, "debug")
	flag.Parse()
	if cfgPath == "" {
		flag.Usage()
		return
	}

	b, fileErr := ioutil.ReadFile(cfgPath)
	if fileErr != nil {
		panic(fileErr)
	}

	yamlCommandSets := mapper.YamlCommandSets{}
	yamlErr := yaml.Unmarshal(b, &yamlCommandSets)
	if yamlErr != nil {
		panic(yamlErr)
	}

	logger := helper.NewBasicLogger(getLogLevel())
	commandSets := yamlCommandSets.BuildCommandSets()

	if isDebug {
		yamlCommandSetsJsonStr, _ := json.MarshalIndent(yamlCommandSets, "", "	")
		logger.Debug(bytes.NewBuffer(yamlCommandSetsJsonStr).String())
		commandSetsJsonStr, _ := json.MarshalIndent(commandSets, "", "	")
		logger.Debug(bytes.NewBuffer(commandSetsJsonStr).String())
	}

	r := runner.NewRunners(commandSets, logger)

	go r.Do()

	done := make(chan bool)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		r.Stop()
		done <- true
	}()

	<-done
}

func getLogLevel() helper.LogLevel {
	result := helper.ERROR
	if isVerbose {
		result = helper.INFO
	}
	if isDebug {
		result = helper.DEBUG
	}
	return result
}
