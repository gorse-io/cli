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

	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var RecommendCmd = &cobra.Command{
	Use:   "recommend",
	Short: "Get recommendations",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
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
