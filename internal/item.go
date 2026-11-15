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
	"strings"

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
