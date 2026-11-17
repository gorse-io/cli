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

var addItemCmd = func() *cobra.Command {
	cmd := &cobra.Command{
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
	cmd.Flags().String("labels", "{}", "Labels as JSON")
	cmd.Flags().String("comment", "", "Comment")
	cmd.Flags().StringArray("category", nil, "Item category, repeatable")
	cmd.Flags().Bool("is-hidden", false, "Hide item from recommendations")
	cmd.Flags().String("timestamp", "", "Item timestamp in RFC3339 format")
	return cmd
}()

var deleteItemCmd = func() *cobra.Command {
	cmd := &cobra.Command{
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
	cmd.Flags().Bool("yes", false, "Confirm deletion (required)")
	return cmd
}()

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
