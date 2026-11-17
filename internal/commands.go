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
	"fmt"
	"os"

	"github.com/gorse-io/gorse/client"
	"github.com/spf13/cobra"
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

func newClient(cmd *cobra.Command) *client.AdminClient {
	endpoint, apiKey := requireEndpointAndKey(cmd)
	return client.NewAdminClient(endpoint, apiKey)
}

func init() {
	ApplicationCmd.AddCommand(
		applicationAddCmd,
		applicationListCmd,
		applicationUseCmd,
		applicationDeleteCmd,
		applicationCurrentCmd,
	)
	CategoryCmd.AddCommand(getCategoriesCmd)
	UserCmd.AddCommand(
		getUserCmd,
		listUserCmd,
		addUserCmd,
		deleteUserCmd,
	)
	ItemCmd.AddCommand(
		getItemCmd,
		listItemCmd,
		addItemCmd,
		deleteItemCmd,
	)
	FeedbackCmd.AddCommand(getFeedbackCmd)
	RecommendCmd.AddCommand(
		recommendUserCmd,
		getLatestCmd,
		getNonPersonalizedCmd,
		getItemToItemCmd,
		getUserToUserCmd,
	)

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
