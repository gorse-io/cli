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

	"github.com/gorse-io/gorse/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var PipelineCmd = &cobra.Command{
	Use: "pipeline", Short: "Manage the recommendation pipeline", Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

func writePipelineYAML(cmd *cobra.Command, value any) error {
	encoder := yaml.NewEncoder(cmd.OutOrStdout())
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return err
	}
	return encoder.Close()
}

var pipelineGetCmd = &cobra.Command{
	Use: "get", Short: "Get recommendation pipeline configuration", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).Get()
		if err != nil {
			return err
		}
		return writePipelineYAML(cmd, formatConfigMap(value))
	},
}
var pipelineSchemaCmd = &cobra.Command{
	Use: "schema", Short: "Get recommendation pipeline configuration schema", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).Schema()
		if err != nil {
			return err
		}
		return writePipelineYAML(cmd, value)
	},
}
var pipelinePatchCmd = &cobra.Command{
	Use: "patch <json-patch>", Short: "Patch recommendation pipeline configuration values", Args: cobra.ExactArgs(1),
	Example: `  gorse-cli pipeline patch '[{"op":"replace","path":"/cache_size","value":1000}]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).Patch([]byte(args[0]))
		if err != nil {
			return err
		}
		printStruct(cmd, value)
		return nil
	},
}
var pipelineResetCmd = &cobra.Command{
	Use: "reset", Short: "Reset recommendation pipeline configuration to the file defaults", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprint(cmd.OutOrStdout(), "Reset recommendation pipeline configuration to file defaults? Current pipeline settings will be overwritten. Confirm [y/N]: ")
		input, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("failed to read confirmation %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(input), "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Pipeline reset canceled")
			return nil
		}
		value, err := NewPipeline(newClient(cmd)).Reset()
		if err != nil {
			return err
		}
		if len(value) > 0 {
			printStruct(cmd, value)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Pipeline reset to defaults.")
		}
		return nil
	},
}

func newPipelineLeaf(use, short string, args cobra.PositionalArgs, run func(*cobra.Command, []string) error) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Args: args, RunE: run}
	cmd.Flags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
	cmd.Flags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
	cmd.Flags().String("application", "", "Gorse CLI application name")
	return cmd
}
func pipelineStringPatch(cmd *cobra.Command, name string) *string {
	if !cmd.Flags().Changed(name) {
		return nil
	}
	value, _ := cmd.Flags().GetString(name)
	return &value
}
func pipelineHasChanges(cmd *cobra.Command, names ...string) error {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return nil
		}
	}
	return fmt.Errorf("no changes supplied")
}
func pipelineScript(cmd *cobra.Command) (*string, error) {
	if cmd.Flags().Changed("script-file") {
		path, _ := cmd.Flags().GetString("script-file")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		value := string(data)
		return &value, nil
	}
	return pipelineStringPatch(cmd, "script"), nil
}

// Each resource supplies typed service calls; this helper only assembles Cobra
// leaves and their common authentication, argument and output handling.
func newPipelineResource(kind string, fields []string, run func(*cobra.Command, string, string) (any, error)) *cobra.Command {
	group := &cobra.Command{
		Use: kind, Short: "Manage " + kind + " recommenders", Args: cobra.NoArgs,
		Long: "Manage " + kind + " recommenders. Adding a recommender does not change explicit ranker or fallback references. Deletion is rejected while such references exist.",
	}
	for _, operation := range []string{"list", "get", "add", "update", "delete"} {
		use := operation + " <name>"
		args := cobra.ExactArgs(1)
		if operation == "list" {
			use = operation
			args = cobra.NoArgs
		}
		leaf := newPipelineLeaf(use, operation+" "+kind+" recommender configuration", args, func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if operation == "update" {
				if err := pipelineHasChanges(cmd, fields...); err != nil {
					return err
				}
			}
			value, err := run(cmd, operation, name)
			if err != nil {
				return err
			}
			if operation == "get" || operation == "list" {
				return writePipelineYAML(cmd, value)
			}
			fmt.Fprintln(cmd.OutOrStdout(), operation+" succeeded:", name)
			return nil
		})
		if operation == "add" || operation == "update" {
			for _, field := range fields {
				leaf.Flags().String(field, "", "Recommender "+field)
			}
			if kind == "external" {
				leaf.MarkFlagsMutuallyExclusive("script", "script-file")
				if operation == "add" {
					leaf.MarkFlagsOneRequired("script", "script-file")
				}
			}
			if operation == "add" {
				switch kind {
				case "item-to-item", "user-to-user":
					_ = leaf.MarkFlagRequired("type")
				case "non-personalized":
					_ = leaf.MarkFlagRequired("score")
				}
			}
		}
		group.AddCommand(leaf)
	}
	return group
}

func newPipelineItemToItemCmd() *cobra.Command {
	return newPipelineResource("item-to-item", []string{"type", "column"}, func(cmd *cobra.Command, operation, name string) (any, error) {
		p := NewPipeline(newClient(cmd))
		switch operation {
		case "list":
			return p.ListItemToItem()
		case "get":
			return p.GetItemToItem(name)
		case "add":
			typ, _ := cmd.Flags().GetString("type")
			column, _ := cmd.Flags().GetString("column")
			return nil, p.AddItemToItem(config.ItemToItemConfig{Name: name, Type: typ, Column: column})
		case "update":
			return nil, p.UpdateItemToItem(name, ItemToItemPatch{Type: pipelineStringPatch(cmd, "type"), Column: pipelineStringPatch(cmd, "column")})
		default:
			return nil, p.DeleteItemToItem(name)
		}
	})
}
func newPipelineUserToUserCmd() *cobra.Command {
	return newPipelineResource("user-to-user", []string{"type", "column"}, func(cmd *cobra.Command, operation, name string) (any, error) {
		p := NewPipeline(newClient(cmd))
		switch operation {
		case "list":
			return p.ListUserToUser()
		case "get":
			return p.GetUserToUser(name)
		case "add":
			typ, _ := cmd.Flags().GetString("type")
			column, _ := cmd.Flags().GetString("column")
			return nil, p.AddUserToUser(config.UserToUserConfig{Name: name, Type: typ, Column: column})
		case "update":
			return nil, p.UpdateUserToUser(name, UserToUserPatch{Type: pipelineStringPatch(cmd, "type"), Column: pipelineStringPatch(cmd, "column")})
		default:
			return nil, p.DeleteUserToUser(name)
		}
	})
}
func newPipelineNonPersonalizedCmd() *cobra.Command {
	return newPipelineResource("non-personalized", []string{"score", "filter"}, func(cmd *cobra.Command, operation, name string) (any, error) {
		p := NewPipeline(newClient(cmd))
		switch operation {
		case "list":
			return p.ListNonPersonalized()
		case "get":
			return p.GetNonPersonalized(name)
		case "add":
			score, _ := cmd.Flags().GetString("score")
			filter, _ := cmd.Flags().GetString("filter")
			return nil, p.AddNonPersonalized(config.NonPersonalizedConfig{Name: name, Score: score, Filter: filter})
		case "update":
			return nil, p.UpdateNonPersonalized(name, NonPersonalizedPatch{Score: pipelineStringPatch(cmd, "score"), Filter: pipelineStringPatch(cmd, "filter")})
		default:
			return nil, p.DeleteNonPersonalized(name)
		}
	})
}
func newPipelineExternalCmd() *cobra.Command {
	return newPipelineResource("external", []string{"script", "script-file"}, func(cmd *cobra.Command, operation, name string) (any, error) {
		p := NewPipeline(newClient(cmd))
		switch operation {
		case "list":
			return p.ListExternal()
		case "get":
			return p.GetExternal(name)
		case "add", "update":
			script, err := pipelineScript(cmd)
			if err != nil {
				return nil, err
			}
			if operation == "update" {
				return nil, p.UpdateExternal(name, ExternalPatch{Script: script})
			}
			value := config.ExternalConfig{Name: name}
			if script != nil {
				value.Script = *script
			}
			return nil, p.AddExternal(value)
		default:
			return nil, p.DeleteExternal(name)
		}
	})
}
func newPipelineCollaborativeCmd() *cobra.Command {
	group := &cobra.Command{Use: "collaborative", Short: "Manage collaborative filtering", Args: cobra.NoArgs}
	get := newPipelineLeaf("get", "Get collaborative filtering configuration", cobra.NoArgs, func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).GetCollaborative()
		if err != nil {
			return err
		}
		return writePipelineYAML(cmd, value)
	})
	update := newPipelineLeaf("update", "Update collaborative filtering configuration", cobra.NoArgs, func(cmd *cobra.Command, args []string) error {
		if err := pipelineHasChanges(cmd, "type", "fit-period", "fit-epoch", "optimize-period", "optimize-trials"); err != nil {
			return err
		}
		patch := CollaborativePatch{Type: pipelineStringPatch(cmd, "type")}
		if cmd.Flags().Changed("fit-period") {
			value, _ := cmd.Flags().GetDuration("fit-period")
			patch.FitPeriod = &value
		}
		if cmd.Flags().Changed("fit-epoch") {
			value, _ := cmd.Flags().GetInt("fit-epoch")
			patch.FitEpoch = &value
		}
		if cmd.Flags().Changed("optimize-period") {
			value, _ := cmd.Flags().GetDuration("optimize-period")
			patch.OptimizePeriod = &value
		}
		if cmd.Flags().Changed("optimize-trials") {
			value, _ := cmd.Flags().GetInt("optimize-trials")
			patch.OptimizeTrials = &value
		}
		if err := NewPipeline(newClient(cmd)).UpdateCollaborative(patch); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Updated collaborative")
		return nil
	})
	update.Flags().String("type", "", "Collaborative filtering type: none or mf")
	update.Flags().Duration("fit-period", 0, "Model fitting period (e.g. 1h)")
	update.Flags().Int("fit-epoch", 0, "Number of fitting epochs")
	update.Flags().Duration("optimize-period", 0, "Model optimization period (0s disables optimization)")
	update.Flags().Int("optimize-trials", 0, "Number of optimization trials")
	group.AddCommand(get, update)
	return group
}
