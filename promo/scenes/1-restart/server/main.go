package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

const version = "v1"

func main() {
	ln, err := net.Listen("tcp", ":8090")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("server %s: listening on :8090\n", version)
	go http.Serve(ln, nil)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	fmt.Printf("server %s: shutting down\n", version)
}
