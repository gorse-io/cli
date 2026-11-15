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

	adminclient "github.com/gorse-io/gorse/client"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var FeedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Manage feedback",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
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

func init() {
	FeedbackCmd.AddCommand(getFeedbackCmd)
	getFeedbackCmd.Flags().IntP("n", "n", 0, "Number of returned feedback")
	getFeedbackCmd.Flags().String("type", "", "Filter by feedback type")
	getFeedbackCmd.Flags().String("user", "", "Filter by user ID")
	getFeedbackCmd.Flags().String("item", "", "Filter by item ID")
	addClientFlags(getFeedbackCmd)
}
