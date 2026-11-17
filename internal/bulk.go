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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/spf13/cobra"
)

const importBatchSize = 1000

func bulkRequest(cmd *cobra.Command, method, path string, body io.Reader) (*http.Response, error) {
	endpoint, apiKey := requireEndpointAndKey(cmd)
	request, err := http.NewRequestWithContext(cmd.Context(), method, strings.TrimRight(endpoint, "/")+"/api/"+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create API request: %w", err)
	}
	request.Header.Set("X-API-Key", apiKey)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("API request failed: status=%d body=%s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	return response, nil
}

func importData(cmd *cobra.Command, args []string, resource string) error {
	input := cmd.InOrStdin()
	if len(args) > 0 && args[0] != "-" {
		file, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open import file: %w", err)
		}
		defer file.Close()
		input = file
	}
	decoder := json.NewDecoder(input)
	batch := make([]json.RawMessage, 0, importBatchSize)
	imported, read := 0, 0
	sendBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		body, err := json.Marshal(batch)
		if err != nil {
			return err
		}
		method := http.MethodPost
		if resource == "feedback" {
			// PUT preserves labels/comments and overwrites existing feedback.
			// The server's legacy bulk feedback importer discards labels.
			method = http.MethodPut
		}
		response, err := bulkRequest(cmd, method, resource, bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer response.Body.Close()
		var result gorse.RowAffected
		if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to decode import response: %w", err)
		}
		imported += result.RowAffected
		batch = batch[:0]
		return nil
	}
	for {
		var record json.RawMessage
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("import failed after %d records: invalid JSONL record %d: %w", imported, read+1, err)
		}
		if len(record) == 0 || record[0] != '{' {
			return fmt.Errorf("import failed after %d records: JSONL record %d must be an object", imported, read+1)
		}
		read++
		batch = append(batch, record)
		if len(batch) == importBatchSize {
			if err = sendBatch(); err != nil {
				return fmt.Errorf("import failed after %d records: %w", imported, err)
			}
		}
	}
	if err := sendBatch(); err != nil {
		return fmt.Errorf("import failed after %d records: %w", imported, err)
	}
	printStruct(cmd, gorse.RowAffected{RowAffected: imported})
	return nil
}

func exportData(cmd *cobra.Command, args []string, resource string) error {
	response, err := bulkRequest(cmd, http.MethodGet, "bulk/"+resource, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if len(args) == 0 || args[0] == "-" {
		if _, err = io.Copy(cmd.OutOrStdout(), response.Body); err != nil {
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
	if _, err = io.Copy(file, response.Body); err != nil {
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
