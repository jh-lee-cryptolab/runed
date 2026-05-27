// rundemo is a tiny consumer of the runed client library for smoke testing.
//
// Usage (with daemon running at ~/.runed/embedding.sock):
//
//	rundemo                         # embed the default sample text
//	rundemo "arbitrary text here"   # embed a caller-provided string
//
// Exit codes: 0 on success, 1 on any error. Errors go to stderr.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/CryptoLabInc/runed/client"
)

func main() {
	text := "the quick brown fox jumps over the lazy dog"
	if len(os.Args) > 1 {
		text = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.Connect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	info, err := c.Info(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "info: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("daemon version: %s\nmodel identity: %s\nvector dim:     %d\n",
		info.GetDaemonVersion(), info.GetModelIdentity(), info.GetVectorDim())

	vec, err := c.Embed(ctx, text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "embed: %v\n", err)
		os.Exit(1)
	}
	if len(vec) < 5 {
		fmt.Fprintf(os.Stderr, "unexpected short vector: len=%d\n", len(vec))
		os.Exit(1)
	}
	fmt.Printf("text:           %q\nvector[0..4]:   %v\nvector length:  %d\nOK\n",
		text, vec[:5], len(vec))
}
