// Copyright 2026 gorse Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gorse-io/gorse/client"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var PsCmd = &cobra.Command{
	Use:   "ps",
	Short: "List task progress",
	Run: func(cmd *cobra.Command, args []string) {
		tasks, err := newClient(cmd).GetTasks()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		t := table.NewWriter()
		t.SetStyle(table.Style{Box: table.BoxStyle{PaddingRight: "  "}})
		t.SuppressTrailingSpaces()
		t.AppendHeader(table.Row{"NAME", "STATUS", "PROGRESS", "START-TIME", "FINISH-TIME"})
		for _, task := range tasks {
			progress := task.Error
			if progress == "" {
				progress = formatProgressBar(task.Count, task.Total)
			}
			t.AppendRow(table.Row{task.Name, task.Status, progress, formatTimestamp(task.StartTime), formatTimestamp(task.FinishTime)})
		}
		lo.Must(fmt.Fprintln(cmd.OutOrStdout(), t.Render()))
	},
}

var DumpCmd = &cobra.Command{
	Use:   "dump <file>",
	Short: "Dump all Gorse data as a binary backup",
	Example: `  # Dump data to a backup file
  gorse-cli dump backup.bin`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputPath := args[0]
		file, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to create dump file %q: %v\n", outputPath, err)
			os.Exit(1)
		}
		defer file.Close()
		if err = newClient(cmd).Dump(file); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Data dumped to "+outputPath)
	},
}

var RestoreCmd = &cobra.Command{
	Use:   "restore <file>",
	Short: "Restore Gorse data from a binary backup",
	Example: `  # Restore data from a backup file
 gorse-cli restore backup.bin`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "Restore data from %s? Existing users, items, feedback, and cache will be overwritten. Confirm [y/N]: ", args[0])
		input, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read confirmation: %v\n", err)
			os.Exit(1)
		}
		if !strings.EqualFold(strings.TrimSpace(input), "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Restore canceled")
			return
		}
		file, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to open restore file %q: %v\n", args[0], err)
			os.Exit(1)
		}
		defer file.Close()
		stats, err := newClient(cmd).Restore(file)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Restored %d users, %d items, %d feedback in %s.\n",
			stats.Users, stats.Items, stats.Feedback, stats.Duration)
	},
}

var GetStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Get global statistics",
	Run: func(cmd *cobra.Command, args []string) {
		stats, err := newClient(cmd).GetStats()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		output := cmd.OutOrStdout()
		fmt.Fprintf(output, "BinaryVersion:          %s\n", stats.BinaryVersion)
		fmt.Fprintf(output, "LatestItemsUpdateTime:  %s\n", formatTimestamp(stats.LatestItemsUpdateTime))
		fmt.Fprintf(output, "MatchingModelFitTime:   %s\n", formatTimestamp(stats.MatchingModelFitTime))
		fmt.Fprintf(output, "MatchingModelScore:\n%s\n", lo.Must(json.MarshalIndent(stats.MatchingModelScore, "", "\t")))
		fmt.Fprintf(output, "NumItemLabels:          %d\n", stats.NumItemLabels)
		fmt.Fprintf(output, "NumItems:               %d\n", stats.NumItems)
		fmt.Fprintf(output, "NumServers:             %d\n", stats.NumServers)
		fmt.Fprintf(output, "NumTotalPosFeedback:    %d\n", stats.NumTotalPosFeedback)
		fmt.Fprintf(output, "NumUserLabels:          %d\n", stats.NumUserLabels)
		fmt.Fprintf(output, "NumUsers:               %d\n", stats.NumUsers)
		fmt.Fprintf(output, "NumValidNegFeedback:    %d\n", stats.NumValidNegFeedback)
		fmt.Fprintf(output, "NumValidPosFeedback:    %d\n", stats.NumValidPosFeedback)
		fmt.Fprintf(output, "NumWorkers:             %d\n", stats.NumWorkers)
		fmt.Fprintf(output, "PopularItemsUpdateTime: %s\n", formatTimestamp(stats.PopularItemsUpdateTime))
		fmt.Fprintf(output, "RankingModelFitTime:    %s\n", formatTimestamp(stats.RankingModelFitTime))
		fmt.Fprintf(output, "RankingModelScore:\n%s\n", lo.Must(json.MarshalIndent(stats.RankingModelScore, "", "\t")))
	},
}

func requireEndpointAndKey(cmd *cobra.Command) (string, string) {
	endpoint, apiKey := getEndpointAndKey(cmd)
	if endpoint == "" || apiKey == "" {
		switch {
		case endpoint == "" && apiKey == "":
			output := cmd.ErrOrStderr()
			fmt.Fprintln(output, "Error: no Gorse application is selected and no endpoint/API key was provided.")
			fmt.Fprintln(output)
			fmt.Fprintln(output, "Create an application:")
			fmt.Fprintln(output, "  gorse-cli application add dev --endpoint http://localhost:8088")
			fmt.Fprintln(output, "Or use environment variables:")
			fmt.Fprintln(output, "  GORSE_ADMIN_ENDPOINT=http://localhost:8088 GORSE_ADMIN_API_KEY=<api-key> "+cmd.CommandPath())
			fmt.Fprintln(output, "Or pass credentials for this command:")
			fmt.Fprintln(output, "  "+cmd.CommandPath()+" --endpoint http://localhost:8088 --api-key <api-key>")
			os.Exit(1)
		case endpoint == "":
			output := cmd.ErrOrStderr()
			fmt.Fprintln(output, "Error: missing Gorse base URL.")
			fmt.Fprintln(output)
			fmt.Fprintln(output, "Use a saved application:")
			fmt.Fprintln(output, "  gorse-cli application use <name>")
			fmt.Fprintln(output, "Or set an environment variable:")
			fmt.Fprintln(output, "  GORSE_ADMIN_ENDPOINT=http://localhost:8088 "+cmd.CommandPath())
			fmt.Fprintln(output, "Or pass an endpoint:")
			fmt.Fprintln(output, "  "+cmd.CommandPath()+" --endpoint http://localhost:8088")
			os.Exit(1)
		case apiKey == "":
			output := cmd.ErrOrStderr()
			fmt.Fprintln(output, "Error: missing Gorse admin API key.")
			fmt.Fprintln(output)
			fmt.Fprintln(output, "Save it in an application:")
			fmt.Fprintln(output, "  gorse-cli application add <name> --endpoint http://localhost:8088")
			fmt.Fprintln(output, "Or set an environment variable:")
			fmt.Fprintln(output, "  GORSE_ADMIN_API_KEY=<api-key> "+cmd.CommandPath())
			fmt.Fprintln(output, "Or pass it for this command:")
			fmt.Fprintln(output, "  "+cmd.CommandPath()+" --api-key <api-key>")
			os.Exit(1)
		}
	}
	return endpoint, apiKey
}

func newClient(cmd *cobra.Command) *client.AdminClient {
	endpoint, apiKey := requireEndpointAndKey(cmd)
	return client.NewAdminClient(endpoint, apiKey)
}
