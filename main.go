package main

import (
	"fmt"
	"os"

	"github.com/gorse-io/cli/internal"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gorse-cli",
		Short: "Gorse command line tool for cluster management",
		Run: func(cmd *cobra.Command, args []string) {
			if showVersion, _ := cmd.PersistentFlags().GetBool("version"); showVersion {
				cmd.Println(internal.BuildInfo())
				return
			}
			_ = cmd.Help()
		},
	}
	rootCmd.PersistentFlags().BoolP("version", "v", false, "gorse-cli version")
	rootCmd.AddCommand(internal.ApplicationCmd)
	rootCmd.AddCommand(internal.ItemCmd, internal.UserCmd, internal.FeedbackCmd, internal.CategoryCmd)
	rootCmd.AddCommand(internal.GetClusterCmd)
	rootCmd.AddCommand(internal.GetStatsCmd)
	rootCmd.AddCommand(internal.RecommendCmd)
	rootCmd.AddCommand(internal.PipelineCmd)
	rootCmd.AddCommand(internal.PsCmd)
	rootCmd.AddCommand(internal.DumpCmd)
	rootCmd.AddCommand(internal.RestoreCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
