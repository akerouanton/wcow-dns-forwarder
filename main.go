package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	r := NewResolver("0.0.0.0")
	r.Start()

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM)
	<-sigCh

	r.Stop()
}
