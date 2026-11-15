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

var DumpCmd = &cobra.Command{
	Use:   "dump <file>",
	Short: "Dump all Gorse data as a binary backup",
	Example: `  # Dump data to a backup file
  gorse-cli dump backup.bin`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputPath := args[0]
		file, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to create dump file %q: %v\n", outputPath, err)
			os.Exit(1)
		}
		defer file.Close()
		if err = newClient(cmd).Dump(file); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Data dumped to "+outputPath)
	},
}

func init() {
	addClientFlags(DumpCmd)
}
