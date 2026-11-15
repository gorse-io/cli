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

	jsonpatch "github.com/evanphx/json-patch/v5"
	gorse "github.com/gorse-io/gorse-go"
	adminclient "github.com/gorse-io/gorse/client"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

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

func newClient(cmd *cobra.Command) *adminclient.AdminClient {
	endpoint, apiKey := requireEndpointAndKey(cmd)
	return adminclient.NewAdminClient(endpoint, apiKey)
}

var ItemCmd = &cobra.Command{
	Use:   "item",
	Short: "Manage items",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var UserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var FeedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Manage feedback",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var CategoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Manage item categories",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var RecommendCmd = &cobra.Command{
	Use:   "recommend",
	Short: "Get recommendations",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var PipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Config recommendation pipeline",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// GetClusterCmd gets cluster nodes
var GetClusterCmd = &cobra.Command{
	Use:   "cluster-info",
	Short: "List cluster nodes",
	Run: func(cmd *cobra.Command, args []string) {
		cluster, err := newClient(cmd).GetCluster()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, cluster)
	},
}

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

var pipelineGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get recommendation pipeline configuration",
	Run: func(cmd *cobra.Command, args []string) {
		configValue, err := newClient(cmd).GetConfig()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		configMap := formatConfigMap(configValue)
		_, recommend, ok := getConfigValue(configMap, "recommend", "Recommend")
		if !ok {
			fmt.Fprintln(cmd.ErrOrStderr(), "Error: recommend config not found")
			os.Exit(1)
		}
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		defer encoder.Close()
		if err = encoder.Encode(recommend); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to encode config: %v\n", err)
			os.Exit(1)
		}
	},
}

var pipelineSchemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Get recommendation pipeline configuration schema",
	Run: func(cmd *cobra.Command, args []string) {
		schema, err := newClient(cmd).GetConfigSchema()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		defer encoder.Close()
		if err = encoder.Encode(schema); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to encode schema: %v\n", err)
			os.Exit(1)
		}
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

var pipelinePatchCmd = &cobra.Command{
	Use:   "patch <json-patch>",
	Short: "Patch recommendation pipeline configuration values",
	Example: `  # Replace a single config value
  gorse-cli pipeline patch '[{"op":"replace","path":"/cache_size","value":1000}]'

  # Replace multiple config values
  gorse-cli pipeline patch '[{"op":"replace","path":"/cache_size","value":1000},{"op":"replace","path":"/data_source/item_ttl","value":72}]'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client := newClient(cmd)
		currentConfigMap, err := client.GetConfigMap()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		recommendKey, recommendConfig, ok := getConfigValue(currentConfigMap, "recommend", "Recommend")
		if !ok {
			fmt.Fprintln(cmd.ErrOrStderr(), "Error: recommend config not found")
			os.Exit(1)
		}

		configBytes, err := json.Marshal(recommendConfig)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: failed to encode config: %v\n", err)
			os.Exit(1)
		}
		patch, err := jsonpatch.DecodePatch([]byte(args[0]))
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: failed to decode JSON patch: %v\n", err)
			os.Exit(1)
		}
		patchedConfigBytes, err := patch.Apply(configBytes)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: %v\n", err)
			os.Exit(1)
		}
		var configPatch map[string]any
		if err = json.Unmarshal(patchedConfigBytes, &configPatch); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: failed to decode patched config: %v\n", err)
			os.Exit(1)
		}
		currentConfigMap[recommendKey] = configPatch

		updatedConfig, err := client.UpdateConfig(currentConfigMap)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}

		printStruct(cmd, updatedConfig)
	},
}

var pipelineResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset recommendation pipeline configuration to the file defaults",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(cmd.OutOrStdout(), "Reset recommendation pipeline configuration to file defaults? Current pipeline settings will be overwritten. Confirm [y/N]: ")
		input, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read confirmation: %v\n", err)
			os.Exit(1)
		}
		if !strings.EqualFold(strings.TrimSpace(input), "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Pipeline reset canceled")
			return
		}
		result, err := newClient(cmd).ResetConfig()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		if len(result) > 0 {
			printStruct(cmd, result)
			return
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Pipeline reset to defaults.")
	},
}

var getCategoriesCmd = &cobra.Command{
	Use:   "list",
	Short: "List item categories",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		categories, err := newClient(cmd).GetCategories()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, categories)
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
		printStruct(cmd, stats)
	},
}

var getUserCmd = &cobra.Command{
	Use:   "get <user-id>",
	Short: "Get user details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		user, err := newClient(cmd).GetUser(cmd.Context(), args[0])
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: API request failed: %v\n", err)
			os.Exit(1)
		}
		printStruct(cmd, user)
	},
}

var getItemCmd = &cobra.Command{
	Use:   "get <item-id>",
	Short: "Get item details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		item, err := newClient(cmd).GetItem(cmd.Context(), args[0])
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: API request failed: %v\n", err)
			os.Exit(1)
		}
		printStruct(cmd, item)
	},
}

var getUsersCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		n := lo.Must(cmd.Flags().GetInt("n"))
		if n == 0 {
			n = 10
		}
		users, err := newClient(cmd).GetUsers(cmd.Context(), n, "")
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, users.Users)
	},
}

var getItemsCmd = &cobra.Command{
	Use:   "list [query...]",
	Short: "List or search items",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		n := lo.Must(cmd.Flags().GetInt("n"))
		if n == 0 {
			n = 10
		}
		client := newClient(cmd)
		var (
			items gorse.ItemIterator
			err   error
		)
		if len(args) > 0 {
			items, err = client.SearchItems(cmd.Context(), strings.Join(args, " "), n)
		} else {
			items, err = client.GetItems(cmd.Context(), n, "")
		}
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, items.Items)
	},
}

var getFeedbackCmd = &cobra.Command{
	Use:   "list",
	Short: "List feedback",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		n := lo.Must(cmd.Flags().GetInt("n"))
		if n == 0 {
			n = 10
		}
		feedbackType := lo.Must(cmd.Flags().GetString("type"))
		userID := lo.Must(cmd.Flags().GetString("user"))
		itemID := lo.Must(cmd.Flags().GetString("item"))
		client := newClient(cmd)
		var (
			feedback []adminclient.Feedback
			err      error
		)
		switch {
		case userID != "" && itemID != "" && feedbackType != "":
			record, requestErr := client.GetTypedUserItemFeedback(feedbackType, userID, itemID)
			err = requestErr
			feedback = []adminclient.Feedback{record}
		case userID != "" && itemID != "":
			feedback, err = client.GetUserItemFeedback(userID, itemID)
		case userID != "" && feedbackType != "":
			feedback, err = client.GetTypedUserFeedback(userID, feedbackType)
		case userID != "":
			feedback, err = client.GetUserFeedback(userID)
		case itemID != "" && feedbackType != "":
			feedback, err = client.GetTypedItemFeedback(itemID, feedbackType)
		case itemID != "":
			feedback, err = client.GetItemFeedback(itemID)
		case feedbackType != "":
			iterator, requestErr := client.GetTypedFeedback(feedbackType, n)
			err = requestErr
			feedback = iterator.Feedback
		default:
			iterator, requestErr := client.GetFeedback(n)
			err = requestErr
			feedback = iterator.Feedback
		}
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		sortFeedback(feedback)
		if n > 0 && n < len(feedback) {
			feedback = feedback[:n]
		}
		printArrayTable(cmd, feedback)
	},
}

var getLatestCmd = &cobra.Command{
	Use:   "latest",
	Short: "Get latest items",
	Run: func(cmd *cobra.Command, args []string) {
		categories := lo.Must(cmd.Flags().GetStringArray("category"))
		n := lo.Must(cmd.Flags().GetInt("n"))
		if n == 0 {
			n = 10
		}
		latest, err := newClient(cmd).GetLatest(n, categories)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, latest)
	},
}

var getNonPersonalizedCmd = &cobra.Command{
	Use:   "non-personalized <name>",
	Short: "Get non-personalized recommendations",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		recommendations, err := newClient(cmd).GetNonPersonalized(
			args[0],
			lo.Must(cmd.Flags().GetInt("n")),
			lo.Must(cmd.Flags().GetString("user-id")),
			lo.Must(cmd.Flags().GetStringArray("category")),
		)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, recommendations)
	},
}

var recommendUserCmd = &cobra.Command{
	Use:   "item-to-user <user-id> [recommender] [name]",
	Short: "Get recommendations for a user",
	Args:  cobra.RangeArgs(1, 3),
	Run: func(cmd *cobra.Command, args []string) {
		categories := lo.Must(cmd.Flags().GetStringArray("category"))
		n := lo.Must(cmd.Flags().GetInt("n"))
		if n == 0 {
			n = 10
		}
		var recommender, name string
		if len(args) > 1 {
			recommender = args[1]
		}
		if len(args) > 2 {
			name = args[2]
		}
		recommendations, err := newClient(cmd).GetRecommend(args[0], recommender, name, n, categories)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, recommendations)
	},
}

var getItemToItemCmd = &cobra.Command{
	Use:   "item-to-item <name> <item-id>",
	Short: "Get item-to-item recommendations",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		recommendations, err := newClient(cmd).GetItemToItem(
			args[0],
			args[1],
			lo.Must(cmd.Flags().GetInt("n")),
			lo.Must(cmd.Flags().GetStringArray("category")),
		)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, recommendations)
	},
}

var getUserToUserCmd = &cobra.Command{
	Use:   "user-to-user <name> <user-id>",
	Short: "Get user-to-user recommendations",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		recommendations, err := newClient(cmd).GetUserToUser(args[0], args[1], lo.Must(cmd.Flags().GetInt("n")))
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		printArrayTable(cmd, recommendations)
	},
}

func init() {
	ApplicationCmd.AddCommand(applicationAddCmd)
	ApplicationCmd.AddCommand(applicationListCmd)
	ApplicationCmd.AddCommand(applicationUseCmd)
	ApplicationCmd.AddCommand(applicationDeleteCmd)
	ApplicationCmd.AddCommand(applicationCurrentCmd)
	CategoryCmd.AddCommand(getCategoriesCmd)
	UserCmd.AddCommand(getUserCmd, getUsersCmd)
	ItemCmd.AddCommand(getItemCmd, getItemsCmd)
	FeedbackCmd.AddCommand(getFeedbackCmd)
	RecommendCmd.AddCommand(recommendUserCmd)
	RecommendCmd.AddCommand(getLatestCmd)
	RecommendCmd.AddCommand(getNonPersonalizedCmd)
	RecommendCmd.AddCommand(getItemToItemCmd)
	RecommendCmd.AddCommand(getUserToUserCmd)

	PipelineCmd.AddCommand(pipelineGetCmd)
	PipelineCmd.AddCommand(pipelineSchemaCmd)
	PipelineCmd.AddCommand(pipelinePatchCmd)
	PipelineCmd.AddCommand(pipelineResetCmd)

	applicationAddCmd.Flags().String("endpoint", "", "Gorse base URL (default: GORSE_ADMIN_ENDPOINT)")
	applicationAddCmd.Flags().String("api-key", "", "Gorse admin API key (default: GORSE_ADMIN_API_KEY; prefer prompt to avoid shell history)")
	for _, cmd := range []*cobra.Command{
		GetClusterCmd, getCategoriesCmd, PsCmd,
		GetStatsCmd, getUserCmd, getUsersCmd, getItemCmd, getItemsCmd, getFeedbackCmd, getLatestCmd,
		getNonPersonalizedCmd, recommendUserCmd, getItemToItemCmd, getUserToUserCmd,
		pipelineGetCmd, pipelineSchemaCmd, pipelinePatchCmd, pipelineResetCmd, DumpCmd, RestoreCmd,
	} {
		cmd.Flags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
		cmd.Flags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
		cmd.Flags().String("application", "", "Gorse CLI application name")
	}

	getUsersCmd.Flags().IntP("n", "n", 0, "Number of returned users")
	getItemsCmd.Flags().IntP("n", "n", 0, "Number of returned items")
	getFeedbackCmd.Flags().IntP("n", "n", 0, "Number of returned feedback")
	getFeedbackCmd.Flags().String("type", "", "Filter by feedback type")
	getFeedbackCmd.Flags().String("user", "", "Filter by user ID")
	getFeedbackCmd.Flags().String("item", "", "Filter by item ID")
	getLatestCmd.Flags().IntP("n", "n", 0, "Number of returned records")
	getLatestCmd.Flags().StringArray("category", nil, "Filter by category, repeatable")
	getNonPersonalizedCmd.Flags().IntP("n", "n", 0, "Number of returned records")
	getNonPersonalizedCmd.Flags().StringArray("category", nil, "Filter by category, repeatable")
	getNonPersonalizedCmd.Flags().String("user-id", "", "Remove read items of a user")
	recommendUserCmd.Flags().IntP("n", "n", 0, "Number of returned items")
	recommendUserCmd.Flags().StringArray("category", nil, "Filter by category, repeatable")
	getItemToItemCmd.Flags().IntP("n", "n", 0, "Number of returned records")
	getItemToItemCmd.Flags().StringArray("category", nil, "Filter by category, repeatable")
	getUserToUserCmd.Flags().IntP("n", "n", 0, "Number of returned records")

}
