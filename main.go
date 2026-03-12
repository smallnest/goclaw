package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/smallnest/goclaw/cli"
)

// Version information, populated by goreleaser
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
	BuiltBy = "unknown"
)

func main() {
	// Setup signal handler for dumping goroutine stack traces
	setupStackDumpSignal()

	// Set version in CLI package
	cli.SetVersion(Version)

	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

// setupStackDumpSignal sets up a signal handler for SIGUSR1/SIGQUIT
// to dump all goroutine stack traces without terminating the program.
// This is useful for debugging "stuck" or slow applications.
//
// Usage: kill -SIGUSR1 <pid> or kill -SIGQUIT <pid>
// Based on: https://colobu.com/2016/12/21/how-to-dump-goroutine-stack-traces/
func setupStackDumpSignal() {
	ch := make(chan os.Signal, 1)
	notifyStackDumpSignal(ch)

	go func() {
		for range ch {
			dumpGoroutineStacks()
		}
	}()
}

// dumpGoroutineStacks dumps all goroutine stack traces to stderr.
// It uses runtime.Stack to get stack traces for all goroutines.
func dumpGoroutineStacks() {
	// Get stack traces for all goroutines
	// The buffer size (4MB) should be enough for most applications
	buf := make([]byte, 1<<20) // 1MB per goroutine, can hold up to 4MB
	n := runtime.Stack(buf, true)
	if n == 0 {
		fmt.Fprintln(os.Stderr, "No goroutine stack traces available")
		return
	}

	fmt.Fprintln(os.Stderr, "========== Goroutine Stack Traces ==========")
	fmt.Fprintln(os.Stderr)
	if _, err := os.Stderr.Write(buf[:n]); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing stack traces: %v\n", err)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "========== End of Stack Traces ==========")
}

func GetVersionInfo() string {
	return fmt.Sprintf("goclaw version %s (commit: %s, built at: %s by: %s)", Version, Commit, Date, BuiltBy)
}
