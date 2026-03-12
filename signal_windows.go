//go:build windows

package main

import (
	"os"
	"os/signal"
)

// notifyStackDumpSignal sets up a signal handler for Windows systems.
// This function configures the provided channel to receive interrupt signals
// which can be used to trigger goroutine stack dumps on Windows platforms.
//
// Unlike Unix systems that support SIGUSR1/SIGQUIT, Windows uses os.Interrupt
// (typically triggered by Ctrl+C) for this purpose.
//
// Usage: The signal channel will receive os.Interrupt when Ctrl+C is pressed
func notifyStackDumpSignal(ch chan os.Signal) {
	// On Windows, we use os.Interrupt (Ctrl+C) since SIGUSR1/SIGQUIT don't exist
	signal.Notify(ch, os.Interrupt)
}
