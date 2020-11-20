package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/seungyeop-lee/directory-watcher/runner"
	"gopkg.in/yaml.v2"
)

var (
	cfgPath     string
	commandSets runner.CommandSets
)

func main() {
	fmt.Println("directory-watcher run")

	defer func() {
		if e := recover(); e != nil {
			log.Fatalf("PANIC: %+v", e)
		}
	}()

	flag.StringVar(&cfgPath, "c", "", "config path")
	flag.Parse()
	if cfgPath == "" {
		flag.Usage()
		return
	}

	b, fileErr := ioutil.ReadFile(cfgPath)
	if fileErr != nil {
		panic(fileErr)
	}

	yamlErr := yaml.Unmarshal(b, &commandSets)
	if yamlErr != nil {
		panic(yamlErr)
	}

	r := runner.NewRunners(commandSets)

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
