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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var ItemCmd = &cobra.Command{
	Use:   "item",
	Short: "Manage items",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var addItemCmd = &cobra.Command{
	Use:   "add <item-id>",
	Short: "Add an item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		item := gorse.Item{ItemId: args[0], Comment: lo.Must(cmd.Flags().GetString("comment"))}
		if err := json.Unmarshal([]byte(lo.Must(cmd.Flags().GetString("labels"))), &item.Labels); err != nil {
			return fmt.Errorf("invalid --labels: %w", err)
		}
		item.Categories = lo.Must(cmd.Flags().GetStringArray("category"))
		item.IsHidden = lo.Must(cmd.Flags().GetBool("is-hidden"))
		if timestamp := lo.Must(cmd.Flags().GetString("timestamp")); timestamp != "" {
			var err error
			item.Timestamp, err = time.Parse(time.RFC3339, timestamp)
			if err != nil {
				return fmt.Errorf("invalid --timestamp: %w", err)
			}
		}
		result, err := newClient(cmd).InsertItem(cmd.Context(), item)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		printStruct(cmd, result)
		return nil
	},
}

var updateItemCmd = &cobra.Command{
	Use:   "update <item-id>",
	Short: "Update item fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var patch gorse.ItemPatch
		if cmd.Flags().Changed("comment") {
			patch.Comment = lo.ToPtr(lo.Must(cmd.Flags().GetString("comment")))
		}
		if cmd.Flags().Changed("labels") {
			if err := json.Unmarshal([]byte(lo.Must(cmd.Flags().GetString("labels"))), &patch.Labels); err != nil {
				return fmt.Errorf("invalid --labels: %w", err)
			}
			if patch.Labels == nil {
				return fmt.Errorf("invalid --labels: null does not update labels; use {} or [] to clear")
			}
		}
		if cmd.Flags().Changed("is-hidden") {
			patch.IsHidden = lo.ToPtr(lo.Must(cmd.Flags().GetBool("is-hidden")))
		}
		if cmd.Flags().Changed("category") {
			patch.Categories = lo.Must(cmd.Flags().GetStringArray("category"))
			if lo.Contains(patch.Categories, "") {
				if len(patch.Categories) != 1 {
					return fmt.Errorf("invalid --category: cannot combine an empty value with other categories")
				}
				patch.Categories = []string{}
			}
		}
		if cmd.Flags().Changed("timestamp") {
			timestamp, err := time.Parse(time.RFC3339, lo.Must(cmd.Flags().GetString("timestamp")))
			if err != nil {
				return fmt.Errorf("invalid --timestamp: %w", err)
			}
			patch.Timestamp = &timestamp
		}
		if patch.Comment == nil && patch.Labels == nil && patch.IsHidden == nil && patch.Categories == nil && patch.Timestamp == nil {
			return fmt.Errorf("at least one update flag is required")
		}
		result, err := newClient(cmd).UpdateItem(cmd.Context(), args[0], patch)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		printStruct(cmd, result)
		return nil
	},
}

var deleteItemCmd = &cobra.Command{
	Use:   "delete <item-id>",
	Short: "Delete an item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !lo.Must(cmd.Flags().GetBool("yes")) {
			return fmt.Errorf("deletion requires confirmation with --yes")
		}
		result, err := newClient(cmd).DeleteItem(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		printStruct(cmd, result)
		return nil
	},
}

var itemCategoriesCmd = &cobra.Command{
	Use:   "categories",
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

var listItemCmd = &cobra.Command{
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

var importItemCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import items from JSONL (default: stdin; use - for stdin)",
	Long:  "Import JSONL records in batches, replacing existing records with the same key. Completed batches are not rolled back on failure.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return importData(cmd, args, "items")
	},
}

var exportItemCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export items as JSONL (default: stdout; use - for stdout)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return exportData(cmd, args, "items")
	},
}

func init() {
	ItemCmd.AddCommand(
		getItemCmd,
		listItemCmd,
		addItemCmd,
		importItemCmd,
		exportItemCmd,
		updateItemCmd,
		deleteItemCmd,
		itemCategoriesCmd,
	)

	addItemCmd.Flags().String("labels", "{}", "Labels as JSON")
	addItemCmd.Flags().String("comment", "", "Comment")
	addItemCmd.Flags().StringArray("category", nil, "Item category, repeatable")
	addItemCmd.Flags().Bool("is-hidden", false, "Hide item from recommendations")
	addItemCmd.Flags().String("timestamp", "", "Item timestamp in RFC3339 format")
	updateItemCmd.Flags().String("labels", "", "Replace labels with JSON ({} or [] to clear)")
	updateItemCmd.Flags().String("comment", "", "Replace comment (empty value clears)")
	updateItemCmd.Flags().StringArray("category", nil, "Replace categories, repeatable (empty value clears all)")
	updateItemCmd.Flags().Bool("is-hidden", false, "Hide item from recommendations")
	updateItemCmd.Flags().String("timestamp", "", "Item timestamp in RFC3339 format")
	deleteItemCmd.Flags().Bool("yes", false, "Confirm deletion (required)")
	listItemCmd.Flags().IntP("n", "n", 0, "Number of returned items")
}
