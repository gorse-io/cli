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
	"sort"
	"text/tabwriter"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/gorse-io/gorse/client"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var FeedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Manage feedback",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var addFeedbackCmd = &cobra.Command{
	Use:   "add <feedback-type> <user-id> <item-id>",
	Short: "Add feedback",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		feedback := gorse.Feedback{
			FeedbackType: args[0],
			UserId:       args[1],
			ItemId:       args[2],
			Value:        lo.Must(cmd.Flags().GetFloat64("value")),
			Timestamp:    time.Now(),
		}
		if timestamp := lo.Must(cmd.Flags().GetString("timestamp")); timestamp != "" {
			var err error
			feedback.Timestamp, err = time.Parse(time.RFC3339, timestamp)
			if err != nil {
				return fmt.Errorf("invalid --timestamp: %w", err)
			}
		}
		result, err := newClient(cmd).InsertFeedback(cmd.Context(), []gorse.Feedback{feedback})
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d feedback added\n", result.RowAffected)
		return nil
	},
}

var deleteFeedbackCmd = &cobra.Command{
	Use:   "delete <feedback-type> <user-id> <item-id>",
	Short: "Delete feedback",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !lo.Must(cmd.Flags().GetBool("yes")) {
			return fmt.Errorf("deletion requires confirmation with --yes")
		}
		result, err := newClient(cmd).DeleteFeedback(cmd.Context(), args[0], args[1], args[2])
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d feedback deleted\n", result.RowAffected)
		return nil
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
		c := newClient(cmd)
		var (
			feedback []client.Feedback
			err      error
		)
		switch {
		case userID != "" && itemID != "" && feedbackType != "":
			record, requestErr := c.GetTypedUserItemFeedback(feedbackType, userID, itemID)
			err = requestErr
			feedback = []client.Feedback{record}
		case userID != "" && itemID != "":
			feedback, err = c.GetUserItemFeedback(userID, itemID)
		case userID != "" && feedbackType != "":
			feedback, err = c.GetTypedUserFeedback(userID, feedbackType)
		case userID != "":
			feedback, err = c.GetUserFeedback(userID)
		case itemID != "" && feedbackType != "":
			feedback, err = c.GetTypedItemFeedback(itemID, feedbackType)
		case itemID != "":
			feedback, err = c.GetItemFeedback(itemID)
		case feedbackType != "":
			iterator, requestErr := c.GetTypedFeedback(feedbackType, n)
			err = requestErr
			feedback = iterator.Feedback
		default:
			iterator, requestErr := c.GetFeedback(n)
			err = requestErr
			feedback = iterator.Feedback
		}
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		sort.Slice(feedback, func(i, j int) bool {
			return feedback[i].Timestamp.After(feedback[j].Timestamp)
		})
		if n > 0 && n < len(feedback) {
			feedback = feedback[:n]
		}
		table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
		fmt.Fprintln(table, "TIMESTAMP\tFEEDBACK-TYPE\tUSER-ID\tITEM-ID\tVALUE\tLABELS\tUPDATED\tCOMMENT")
		for _, record := range feedback {
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%v\t%s\t%s\t%s\n",
				record.Timestamp, record.FeedbackType,
				record.UserId, record.ItemId, record.Value,
				formatInlineJSON(record.Labels), record.Updated, record.Comment)
		}
		lo.Must0(table.Flush())
	},
}

var importFeedbackCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import feedback from JSONL (default: stdin; use - for stdin)",
	Long:  "Import JSONL records, replacing existing records with the same key. Completed server-side batches are not rolled back on failure.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient(cmd)
		input := cmd.InOrStdin()
		if len(args) > 0 && args[0] != "-" {
			file, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("failed to open import file: %w", err)
			}
			defer file.Close()
			input = file
		}
		result, err := c.ImportFeedback(input)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d feedback imported\n", result.RowAffected)
		return nil
	},
}

var exportFeedbackCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export feedback as JSONL (default: stdout; use - for stdout)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient(cmd)
		output := cmd.OutOrStdout()
		if len(args) == 0 || args[0] == "-" {
			if err := c.ExportFeedback(output); err != nil {
				return fmt.Errorf("failed to export data: %w", err)
			}
			return nil
		}
		path := args[0]
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("failed to create export file: %w", err)
		}
		exportErr := c.ExportFeedback(file)
		closeErr := file.Close()
		if exportErr != nil {
			return fmt.Errorf("failed to export data: %w", exportErr)
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close export file: %w", closeErr)
		}
		_, err = fmt.Fprintln(output, "Data exported to "+path)
		return err
	},
}

func init() {
	FeedbackCmd.AddCommand(getFeedbackCmd, addFeedbackCmd, deleteFeedbackCmd, importFeedbackCmd, exportFeedbackCmd)

	getFeedbackCmd.Flags().IntP("n", "n", 0, "Number of returned feedback")
	getFeedbackCmd.Flags().String("type", "", "Filter by feedback type")
	getFeedbackCmd.Flags().String("user", "", "Filter by user ID")
	getFeedbackCmd.Flags().String("item", "", "Filter by item ID")

	addFeedbackCmd.Flags().Float64("value", 1, "Feedback value")
	addFeedbackCmd.Flags().String("timestamp", "", "Feedback timestamp in RFC3339 format (default: current time)")
	deleteFeedbackCmd.Flags().Bool("yes", false, "Confirm deletion (required)")
}
