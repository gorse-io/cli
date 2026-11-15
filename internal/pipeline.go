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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var PipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Config recommendation pipeline",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var pipelineGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get recommendation pipeline configuration",
	Run: func(cmd *cobra.Command, args []string) {
		configValue, err := newClient(cmd).GetConfig()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		configMap := formatConfigMap(configValue)
		_, recommend, ok := getConfigValue(configMap, "recommend", "Recommend")
		if !ok {
			fmt.Fprintln(cmd.ErrOrStderr(), "Error: recommend config not found")
			os.Exit(1)
		}
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		defer encoder.Close()
		if err = encoder.Encode(recommend); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to encode config: %v\n", err)
			os.Exit(1)
		}
	},
}

var pipelineSchemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Get recommendation pipeline configuration schema",
	Run: func(cmd *cobra.Command, args []string) {
		schema, err := newClient(cmd).GetConfigSchema()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		defer encoder.Close()
		if err = encoder.Encode(schema); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to encode schema: %v\n", err)
			os.Exit(1)
		}
	},
}

var pipelinePatchCmd = &cobra.Command{
	Use:   "patch <json-patch>",
	Short: "Patch recommendation pipeline configuration values",
	Example: `  # Replace a single config value
  gorse-cli pipeline patch '[{"op":"replace","path":"/cache_size","value":1000}]'

  # Replace multiple config values
  gorse-cli pipeline patch '[{"op":"replace","path":"/cache_size","value":1000},{"op":"replace","path":"/data_source/item_ttl","value":72}]'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client := newClient(cmd)
		currentConfigMap, err := client.GetConfigMap()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		recommendKey, recommendConfig, ok := getConfigValue(currentConfigMap, "recommend", "Recommend")
		if !ok {
			fmt.Fprintln(cmd.ErrOrStderr(), "Error: recommend config not found")
			os.Exit(1)
		}

		configBytes, err := json.Marshal(recommendConfig)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: failed to encode config: %v\n", err)
			os.Exit(1)
		}
		patch, err := jsonpatch.DecodePatch([]byte(args[0]))
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: failed to decode JSON patch: %v\n", err)
			os.Exit(1)
		}
		patchedConfigBytes, err := patch.Apply(configBytes)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: %v\n", err)
			os.Exit(1)
		}
		var configPatch map[string]any
		if err = json.Unmarshal(patchedConfigBytes, &configPatch); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to apply JSON patch: failed to decode patched config: %v\n", err)
			os.Exit(1)
		}
		currentConfigMap[recommendKey] = configPatch

		updatedConfig, err := client.UpdateConfig(currentConfigMap)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}

		printStruct(cmd, updatedConfig)
	},
}

var pipelineResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset recommendation pipeline configuration to the file defaults",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(cmd.OutOrStdout(), "Reset recommendation pipeline configuration to file defaults? Current pipeline settings will be overwritten. Confirm [y/N]: ")
		input, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to read confirmation: %v\n", err)
			os.Exit(1)
		}
		if !strings.EqualFold(strings.TrimSpace(input), "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Pipeline reset canceled")
			return
		}
		result, err := newClient(cmd).ResetConfig()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: admin API request failed: %v\n", err)
			os.Exit(1)
		}
		if len(result) > 0 {
			printStruct(cmd, result)
			return
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Pipeline reset to defaults.")
	},
}

func init() {
	PipelineCmd.AddCommand(pipelineGetCmd)
	PipelineCmd.AddCommand(pipelineSchemaCmd)
	PipelineCmd.AddCommand(pipelinePatchCmd)
	PipelineCmd.AddCommand(pipelineResetCmd)
	addClientFlags(pipelineGetCmd, pipelineSchemaCmd, pipelinePatchCmd, pipelineResetCmd)
}
