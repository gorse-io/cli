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
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var RestoreCmd = &cobra.Command{
	Use:   "restore <file>",
	Short: "Restore Gorse data from a binary backup",
	Example: `  # Restore data from a backup file
 gorse-cli restore backup.bin`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "Restore data from %s? Existing users, items, feedback, and cache will be overwritten. Confirm [y/N]: ", args[0])
		input, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read confirmation: %v\n", err)
			os.Exit(1)
		}
		if !strings.EqualFold(strings.TrimSpace(input), "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Restore canceled")
			return
		}
		file, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to open restore file %q: %v\n", args[0], err)
			os.Exit(1)
		}
		defer file.Close()
		stats, err := newClient(cmd).Restore(file)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Restored %d users, %d items, %d feedback in %s.\n",
			stats.Users, stats.Items, stats.Feedback, stats.Duration)
	},
}

func init() {
	addClientFlags(RestoreCmd)
}
