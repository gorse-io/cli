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
	"net/url"
	"os"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var UserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
	Args:  cobra.NoArgs,
	Run:   func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

var addUserCmd = &cobra.Command{
	Use:   "add <user-id>",
	Short: "Add a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		user := gorse.User{UserId: args[0], Comment: lo.Must(cmd.Flags().GetString("comment"))}
		if err := json.Unmarshal([]byte(lo.Must(cmd.Flags().GetString("labels"))), &user.Labels); err != nil {
			return fmt.Errorf("invalid --labels: %w", err)
		}
		result, err := newClient(cmd).InsertUser(cmd.Context(), user)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d user added\n", result.RowAffected)
		return nil
	},
}

var updateUserCmd = &cobra.Command{
	Use:   "update <user-id>",
	Short: "Update user fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var patch gorse.UserPatch
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
		if patch.Comment == nil && patch.Labels == nil {
			return fmt.Errorf("at least one update flag is required")
		}
		result, err := newClient(cmd).UpdateUser(cmd.Context(), args[0], patch)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d user updated\n", result.RowAffected)
		return nil
	},
}

var deleteUserCmd = &cobra.Command{
	Use:   "delete <user-id>",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !lo.Must(cmd.Flags().GetBool("yes")) {
			return fmt.Errorf("deletion requires confirmation with --yes")
		}
		result, err := newClient(cmd).DeleteUser(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d user deleted\n", result.RowAffected)
		return nil
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
		output := cmd.OutOrStdout()
		fmt.Fprintf(output, "UserId:  %s\n", user.UserId)
		fmt.Fprintf(output, "Comment: %s\n", user.Comment)
		fmt.Fprintf(output, "Labels:\n%s\n", lo.Must(json.MarshalIndent(user.Labels, "", "\t")))
	},
}

var listUserCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		n := lo.Must(cmd.Flags().GetInt("n"))
		if n == 0 {
			n = 10
		}
		cursor := lo.Must(cmd.Flags().GetString("cursor"))
		users, err := newClient(cmd).GetUsers(cmd.Context(), n, url.QueryEscape(cursor))
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		printArrayTable(cmd, users.Users)
		printNextPage(cmd, users.Cursor, n)
		return nil
	},
}

var importUserCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import users from JSONL (default: stdin; use - for stdin)",
	Long:  "Import JSONL records, replacing existing records with the same key. Completed server-side batches are not rolled back on failure.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return importData(cmd, args, "users", newClient(cmd).ImportUsers)
	},
}

var exportUserCmd = &cobra.Command{
	Use:   "export [file]",
	Short: "Export users as JSONL (default: stdout; use - for stdout)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return exportData(cmd, args, newClient(cmd).ExportUsers)
	},
}

func init() {
	UserCmd.AddCommand(
		getUserCmd,
		listUserCmd,
		addUserCmd,
		importUserCmd,
		exportUserCmd,
		updateUserCmd,
		deleteUserCmd,
	)

	addUserCmd.Flags().String("labels", "{}", "Labels as JSON")
	addUserCmd.Flags().String("comment", "", "Comment")
	updateUserCmd.Flags().String("labels", "", "Replace labels with JSON ({} or [] to clear)")
	updateUserCmd.Flags().String("comment", "", "Replace comment (empty value clears)")
	deleteUserCmd.Flags().Bool("yes", false, "Confirm deletion (required)")
	listUserCmd.Flags().IntP("n", "n", 0, "Number of returned users")
	listUserCmd.Flags().String("cursor", "", "Cursor for the next page")
}
