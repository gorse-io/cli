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
	"io"
	"os"
	"path/filepath"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/spf13/cobra"
)

func importData(cmd *cobra.Command, args []string, importFunc func(io.Reader) (gorse.RowAffected, error)) error {
	input := cmd.InOrStdin()
	if len(args) > 0 && args[0] != "-" {
		file, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open import file: %w", err)
		}
		defer file.Close()
		input = file
	}
	result, err := importFunc(input)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	printStruct(cmd, result)
	return nil
}

func exportData(cmd *cobra.Command, args []string, exportFunc func(io.Writer) error) error {
	if len(args) == 0 || args[0] == "-" {
		if err := exportFunc(cmd.OutOrStdout()); err != nil {
			return fmt.Errorf("failed to export data: %w", err)
		}
		return nil
	}
	path := args[0]
	file, err := os.CreateTemp(filepath.Dir(path), ".gorse-export-*")
	if err != nil {
		return fmt.Errorf("failed to create export file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = exportFunc(file); err != nil {
		return fmt.Errorf("failed to export data: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("failed to close export file: %w", err)
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("failed to save export file: %w", err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Data exported to "+path)
	return err
}
