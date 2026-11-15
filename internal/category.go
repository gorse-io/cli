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

	"github.com/spf13/cobra"
)

var CategoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Manage item categories",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
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

func init() {
	CategoryCmd.AddCommand(getCategoriesCmd)
	addClientFlags(getCategoriesCmd)
}
