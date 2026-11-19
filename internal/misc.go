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
	"path/filepath"
	"strings"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/gorse-io/gorse/client"
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
		rows := make([][]string, len(tasks))
		for i, task := range tasks {
			rows[i] = []string{
				formatTableValue(task.Tracer),
				formatTableValue(task.Name),
				formatTableValue(task.Status),
				formatTableValue(formatProgressBar(task.Count, task.Total)),
				formatTableValue(task.Error),
				formatTableValue(task.StartTime),
				formatTableValue(task.FinishTime),
			}
		}
		printTable(cmd.OutOrStdout(), []string{"Tracer", "Name", "Status", "Progress", "Error", "StartTime", "FinishTime"}, rows)
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
		fmt.Fprintf(output, "LatestItemsUpdateTime:  %s\n", stats.LatestItemsUpdateTime.Format(time.RFC3339))
		fmt.Fprintf(output, "MatchingModelFitTime:   %s\n", stats.MatchingModelFitTime.Format(time.RFC3339))
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
		fmt.Fprintf(output, "PopularItemsUpdateTime: %s\n", stats.PopularItemsUpdateTime.Format(time.RFC3339))
		fmt.Fprintf(output, "RankingModelFitTime:    %s\n", stats.RankingModelFitTime.Format(time.RFC3339))
		fmt.Fprintf(output, "RankingModelScore:\n%s\n", lo.Must(json.MarshalIndent(stats.RankingModelScore, "", "\t")))
	},
}

func importData(cmd *cobra.Command, args []string, resource string, importFunc func(io.Reader) (gorse.RowAffected, error)) error {
	input := cmd.InOrStdin()
	if len(args) > 0 && args[0] != "-" {
		file, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open import file: %w", err)
		}
		defer file.Close()
		input = file
	}
	result, err := importFunc(input)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%d %s imported\n", result.RowAffected, resource)
	return nil
}

func exportData(cmd *cobra.Command, args []string, exportFunc func(io.Writer) error) error {
	if len(args) == 0 || args[0] == "-" {
		if err := exportFunc(cmd.OutOrStdout()); err != nil {
			return fmt.Errorf("failed to export data: %w", err)
		}
		return nil
	}
	path := args[0]
	file, err := os.CreateTemp(filepath.Dir(path), ".gorse-export-*")
	if err != nil {
		return fmt.Errorf("failed to create export file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = exportFunc(file); err != nil {
		return fmt.Errorf("failed to export data: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("failed to close export file: %w", err)
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("failed to save export file: %w", err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Data exported to "+path)
	return err
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

func printNextPage(cmd *cobra.Command, cursor string, n int) {
	if cursor == "" {
		return
	}
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	parts := []string{cmd.CommandPath(), "-n", fmt.Sprint(n), "--cursor", quote(cursor)}
	for _, name := range []string{"application", "endpoint"} {
		if cmd.Flags().Changed(name) {
			value, _ := cmd.Flags().GetString(name)
			parts = append(parts, "--"+name, quote(value))
		}
	}
	if cmd.Flags().Changed("api-key") {
		parts = append(parts, "--api-key", quote("<api-key>"))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nNext page: %s\n", strings.Join(parts, " "))
}

func newClient(cmd *cobra.Command) *client.AdminClient {
	endpoint, apiKey := requireEndpointAndKey(cmd)
	return client.NewAdminClient(endpoint, apiKey)
}
