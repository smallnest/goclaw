//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyStackDumpSignal sets up a signal handler for Unix/Linux systems.
// This function configures the provided channel to receive SIGUSR1 and SIGQUIT signals
// which can be used to trigger goroutine stack dumps on Unix/Linux platforms.
//
// Unlike Windows that uses os.Interrupt (Ctrl+C), Unix systems support SIGUSR1/SIGQUIT
// for debugging purposes without terminating the application.
//
// Usage: The signal channel will receive SIGUSR1/SIGQUIT when using kill -SIGUSR1 <pid>
// or kill -SIGQUIT <pid>
func notifyStackDumpSignal(ch chan os.Signal) {
	// On Unix systems, we use SIGUSR1 and SIGQUIT for stack dump signals
	signal.Notify(ch, syscall.SIGUSR1, syscall.SIGQUIT)
}
