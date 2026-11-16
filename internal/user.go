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
		printStruct(cmd, result)
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
		printStruct(cmd, result)
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
		printStruct(cmd, user)
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
