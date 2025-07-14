package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/devlup-labs/Libr/core/mod/config"
	Peers "github.com/devlup-labs/Libr/core/mod/internal/peers"
)

func main() {
	load, _ := config.LoadConfig()
	fmt.Println(load)

	// r := routers.Routers()
	// fmt.Println("Listening on http://localhost:5000")
	// log.Fatal(http.ListenAndServe(":5000", r))

	relayAdd := "/dns4/0.tcp.in.ngrok.io/tcp/13130/p2p/12D3KooWM4RkAQo2RfqpahpnD6hUrz9vCuukPThtUUdocnztEi8P"
	Peers.StartNode(relayAdd)
	sigChan := make(chan os.Signal, 1)

	// Notify on interrupt and terminate signals
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Block until a signal is received
	<-sigChan

	fmt.Println("Interrupt received. Exiting gracefully.")
}
