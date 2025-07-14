package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/devlup-labs/Libr/core/client/handler"
	"github.com/devlup-labs/Libr/core/client/keycache"
	Peers "github.com/devlup-labs/Libr/core/client/peers"
)

func main() {

	relayAdd := "/dns4/0.tcp.in.ngrok.io/tcp/16459/p2p/12D3KooWPyuqvAsuMaRgnxXyMdd84q7VNRsUr2MGj6GqxNkPUwBK"
	Peers.StartNode(relayAdd)
	keycache.InitKeys()
	handler.RunInputLoop()
	sigChan := make(chan os.Signal, 1)

	// Notify on interrupt and terminate signals
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Block until a signal is received
	<-sigChan

	fmt.Println("Interrupt received. Exiting gracefully.")
}
