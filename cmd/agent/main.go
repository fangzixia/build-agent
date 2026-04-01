package main

import (
	"context"
	"fmt"
	"os"

	"build-agent/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	root.SetContext(context.Background())
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
