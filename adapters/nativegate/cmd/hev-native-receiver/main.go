// hev-native-receiver consumes one reference-only request from an authenticated
// local owner. It never enrolls, schedules, issues fences or selects argv.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hev/kit/adapters/nativegate"
)

func main() {
	flags := flag.NewFlagSet("hev-native-receiver", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "Explicit absolute private receiver configuration")
	if flags.Parse(os.Args[1:]) != nil || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "invalid native receiver arguments")
		os.Exit(2)
	}
	cfg, err := nativegate.LoadReceiverConfig(*configPath)
	if err != nil {
		fail()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	// Native Wait deliberately does not terminate an already-started child on
	// context cancellation. A hard supervisor exit bounds this command and leaves
	// the original journal for explicit recovery; it never manufactures a retry.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		select {
		case <-done:
			return
		default:
		}
		fmt.Fprintln(os.Stderr, "native receiver interrupted; inspect original journal before retry")
		os.Exit(124)
	}()
	call, err := nativegate.DecodeReceiverCall(os.Stdin)
	if err != nil {
		fail()
	}
	result, err := nativegate.RunReceiver(ctx, cfg, call)
	if err != nil {
		fail()
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		fail()
	}
}
func fail() {
	fmt.Fprintln(os.Stderr, "native receiver held; inspect original claim and journal")
	os.Exit(1)
}
