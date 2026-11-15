package main

import (
	"fmt"
	"os"

	"github.com/gorse-io/cli/internal"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "gorse-cli",
		Short: "Gorse command line tool for cluster management",
	}
	internal.Register(root)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
