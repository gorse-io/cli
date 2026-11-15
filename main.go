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
	rootCmd.PersistentFlags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
	rootCmd.PersistentFlags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
	rootCmd.PersistentFlags().String("application", "", "Gorse CLI application name")
	rootCmd.AddCommand(
		internal.ApplicationCmd,
		internal.ItemCmd,
		internal.UserCmd,
		internal.FeedbackCmd,
		internal.CategoryCmd,
		internal.GetStatsCmd,
		internal.RecommendCmd,
		internal.PipelineCmd,
		internal.PsCmd,
		internal.DumpCmd,
		internal.RestoreCmd,
	)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
