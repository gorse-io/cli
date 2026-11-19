// Copyright 2021, 2026 gorse Project Authors
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
	"slices"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/gorse-io/gorse/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

var PipelineCmd = &cobra.Command{
	Use: "pipeline", Short: "Manage the recommendation pipeline", Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) { _ = cmd.Help() },
}

func marshal(cmd *cobra.Command, value any) error {
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
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		value, err := encodePipeline(p)
		if err != nil {
			return err
		}
		return marshal(cmd, value)
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
		value, err := newClient(cmd).ResetConfig()
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

func pipelineHasChanges(cmd *cobra.Command, names ...string) error {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return nil
		}
	}
	return fmt.Errorf("no changes supplied")
}

func pipelineScript(cmd *cobra.Command) (string, error) {
	if cmd.Flags().Changed("script-file") {
		path, _ := cmd.Flags().GetString("script-file")
		data, err := os.ReadFile(path)
		return string(data), err
	}
	return cmd.Flags().GetString("script")
}

var pipelineItemToItemCmd = &cobra.Command{
	Use:   "item-to-item",
	Short: "Manage item-to-item recommenders",
	Long:  "Manage item-to-item recommenders. Adding a recommender does not change explicit ranker or fallback references. The server validates configuration changes.",
	Args:  cobra.NoArgs,
}

var pipelineItemToItemListCmd = &cobra.Command{
	Use:   "list",
	Short: "list item-to-item recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		return marshal(cmd, p.ItemToItem)
	},
}

var pipelineItemToItemGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get item-to-item recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("item-to-item", name); err != nil {
			return err
		}
		for _, value := range p.ItemToItem {
			if value.Name == name {
				return marshal(cmd, value)
			}
		}
		return fmt.Errorf("item-to-item %q not found", name)
	},
}

var pipelineItemToItemAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add item-to-item recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		for _, value := range p.ItemToItem {
			if value.Name == name {
				return fmt.Errorf("item-to-item %q already exists", name)
			}
		}
		typ, _ := cmd.Flags().GetString("type")
		column, _ := cmd.Flags().GetString("column")
		p.ItemToItem = append(p.ItemToItem, ItemToItemConfig{Name: name, Type: typ, Column: column})
		if err := p.Save(c); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", name)
		return nil
	},
}

var pipelineItemToItemUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "update item-to-item recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := pipelineHasChanges(cmd, "type", "column"); err != nil {
			return err
		}
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("item-to-item", name); err != nil {
			return err
		}
		for i, value := range p.ItemToItem {
			if value.Name != name {
				continue
			}
			if cmd.Flags().Changed("type") {
				p.ItemToItem[i].Type, _ = cmd.Flags().GetString("type")
			}
			if cmd.Flags().Changed("column") {
				p.ItemToItem[i].Column, _ = cmd.Flags().GetString("column")
			}
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", name)
			return nil
		}
		return fmt.Errorf("item-to-item %q not found", name)
	},
}

var pipelineItemToItemDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete item-to-item recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("item-to-item", name); err != nil {
			return err
		}
		for i, value := range p.ItemToItem {
			if value.Name != name {
				continue
			}
			p.ItemToItem = slices.Delete(p.ItemToItem, i, i+1)
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", name)
			return nil
		}
		return fmt.Errorf("item-to-item %q not found", name)
	},
}

var pipelineUserToUserCmd = &cobra.Command{
	Use:   "user-to-user",
	Short: "Manage user-to-user recommenders",
	Long:  "Manage user-to-user recommenders. Adding a recommender does not change explicit ranker or fallback references. The server validates configuration changes.",
	Args:  cobra.NoArgs,
}

var pipelineUserToUserListCmd = &cobra.Command{
	Use:   "list",
	Short: "list user-to-user recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		return marshal(cmd, p.UserToUser)
	},
}

var pipelineUserToUserGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get user-to-user recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("user-to-user", name); err != nil {
			return err
		}
		for _, value := range p.UserToUser {
			if value.Name == name {
				return marshal(cmd, value)
			}
		}
		return fmt.Errorf("user-to-user %q not found", name)
	},
}

var pipelineUserToUserAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add user-to-user recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		for _, value := range p.UserToUser {
			if value.Name == name {
				return fmt.Errorf("user-to-user %q already exists", name)
			}
		}
		typ, _ := cmd.Flags().GetString("type")
		column, _ := cmd.Flags().GetString("column")
		p.UserToUser = append(p.UserToUser, UserToUserConfig{Name: name, Type: typ, Column: column})
		if err := p.Save(c); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", name)
		return nil
	},
}

var pipelineUserToUserUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "update user-to-user recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := pipelineHasChanges(cmd, "type", "column"); err != nil {
			return err
		}
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("user-to-user", name); err != nil {
			return err
		}
		for i, value := range p.UserToUser {
			if value.Name != name {
				continue
			}
			if cmd.Flags().Changed("type") {
				p.UserToUser[i].Type, _ = cmd.Flags().GetString("type")
			}
			if cmd.Flags().Changed("column") {
				p.UserToUser[i].Column, _ = cmd.Flags().GetString("column")
			}
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", name)
			return nil
		}
		return fmt.Errorf("user-to-user %q not found", name)
	},
}

var pipelineUserToUserDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete user-to-user recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("user-to-user", name); err != nil {
			return err
		}
		for i, value := range p.UserToUser {
			if value.Name != name {
				continue
			}
			p.UserToUser = slices.Delete(p.UserToUser, i, i+1)
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", name)
			return nil
		}
		return fmt.Errorf("user-to-user %q not found", name)
	},
}

var pipelineNonPersonalizedCmd = &cobra.Command{
	Use:   "non-personalized",
	Short: "Manage non-personalized recommenders",
	Long:  "Manage non-personalized recommenders. Adding a recommender does not change explicit ranker or fallback references. The server validates configuration changes.",
	Args:  cobra.NoArgs,
}

var pipelineNonPersonalizedListCmd = &cobra.Command{
	Use:   "list",
	Short: "list non-personalized recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		return marshal(cmd, p.NonPersonalized)
	},
}

var pipelineNonPersonalizedGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get non-personalized recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("non-personalized", name); err != nil {
			return err
		}
		for _, value := range p.NonPersonalized {
			if value.Name == name {
				return marshal(cmd, value)
			}
		}
		return fmt.Errorf("non-personalized %q not found", name)
	},
}

var pipelineNonPersonalizedAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add non-personalized recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		for _, value := range p.NonPersonalized {
			if value.Name == name {
				return fmt.Errorf("non-personalized %q already exists", name)
			}
		}
		score, _ := cmd.Flags().GetString("score")
		filter, _ := cmd.Flags().GetString("filter")
		p.NonPersonalized = append(p.NonPersonalized, NonPersonalizedConfig{Name: name, Score: score, Filter: filter})
		if err := p.Save(c); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", name)
		return nil
	},
}

var pipelineNonPersonalizedUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "update non-personalized recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := pipelineHasChanges(cmd, "score", "filter"); err != nil {
			return err
		}
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("non-personalized", name); err != nil {
			return err
		}
		for i, value := range p.NonPersonalized {
			if value.Name != name {
				continue
			}
			if cmd.Flags().Changed("score") {
				p.NonPersonalized[i].Score, _ = cmd.Flags().GetString("score")
			}
			if cmd.Flags().Changed("filter") {
				p.NonPersonalized[i].Filter, _ = cmd.Flags().GetString("filter")
			}
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", name)
			return nil
		}
		return fmt.Errorf("non-personalized %q not found", name)
	},
}

var pipelineNonPersonalizedDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete non-personalized recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("non-personalized", name); err != nil {
			return err
		}
		for i, value := range p.NonPersonalized {
			if value.Name != name {
				continue
			}
			p.NonPersonalized = slices.Delete(p.NonPersonalized, i, i+1)
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", name)
			return nil
		}
		return fmt.Errorf("non-personalized %q not found", name)
	},
}

var pipelineExternalCmd = &cobra.Command{
	Use:   "external",
	Short: "Manage external recommenders",
	Long:  "Manage external recommenders. Adding a recommender does not change explicit ranker or fallback references. The server validates configuration changes.",
	Args:  cobra.NoArgs,
}

var pipelineExternalListCmd = &cobra.Command{
	Use:   "list",
	Short: "list external recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		return marshal(cmd, p.External)
	},
}

var pipelineExternalGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get external recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("external", name); err != nil {
			return err
		}
		for _, value := range p.External {
			if value.Name == name {
				return marshal(cmd, value)
			}
		}
		return fmt.Errorf("external %q not found", name)
	},
}

var pipelineExternalAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add external recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		script, err := pipelineScript(cmd)
		if err != nil {
			return err
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		for _, value := range p.External {
			if value.Name == name {
				return fmt.Errorf("external %q already exists", name)
			}
		}
		p.External = append(p.External, ExternalConfig{Name: name, Script: script})
		if err := p.Save(c); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", name)
		return nil
	},
}

var pipelineExternalUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "update external recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := pipelineHasChanges(cmd, "script", "script-file"); err != nil {
			return err
		}
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		script, err := pipelineScript(cmd)
		if err != nil {
			return err
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("external", name); err != nil {
			return err
		}
		for i, value := range p.External {
			if value.Name != name {
				continue
			}
			p.External[i].Script = script
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", name)
			return nil
		}
		return fmt.Errorf("external %q not found", name)
	},
}

var pipelineExternalDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete external recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("name must not be empty")
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if err := p.requireUnambiguousName("external", name); err != nil {
			return err
		}
		for i, value := range p.External {
			if value.Name != name {
				continue
			}
			p.External = slices.Delete(p.External, i, i+1)
			if err := p.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", name)
			return nil
		}
		return fmt.Errorf("external %q not found", name)
	},
}

var pipelineCollaborativeCmd = &cobra.Command{
	Use: "collaborative", Short: "Manage collaborative filtering", Args: cobra.NoArgs,
}

var pipelineCollaborativeGetCmd = &cobra.Command{
	Use: "get", Short: "Get collaborative filtering configuration", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var p Pipeline
		if err := p.Load(newClient(cmd)); err != nil {
			return err
		}
		return marshal(cmd, p.Collaborative)
	},
}

var pipelineCollaborativeUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update collaborative filtering configuration", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := pipelineHasChanges(cmd, "type", "fit-period", "fit-epoch", "optimize-period", "optimize-trials"); err != nil {
			return err
		}
		c := newClient(cmd)
		var p Pipeline
		if err := p.Load(c); err != nil {
			return err
		}
		if cmd.Flags().Changed("type") {
			p.Collaborative.Type, _ = cmd.Flags().GetString("type")
		}
		if cmd.Flags().Changed("fit-period") {
			p.Collaborative.FitPeriod, _ = cmd.Flags().GetDuration("fit-period")
		}
		if cmd.Flags().Changed("fit-epoch") {
			p.Collaborative.FitEpoch, _ = cmd.Flags().GetInt("fit-epoch")
		}
		if cmd.Flags().Changed("optimize-period") {
			p.Collaborative.OptimizePeriod, _ = cmd.Flags().GetDuration("optimize-period")
		}
		if cmd.Flags().Changed("optimize-trials") {
			p.Collaborative.OptimizeTrials, _ = cmd.Flags().GetInt("optimize-trials")
		}
		if err := p.Save(c); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Updated collaborative")
		return nil
	},
}

var pipelineUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update global recommendation settings", Args: cobra.NoArgs,
	RunE: pipelineUpdateSettings,
}

var pipelineDataSourceCmd = &cobra.Command{
	Use: "data-source", Short: "Manage recommendation data sources", Args: cobra.NoArgs,
}
var pipelineDataSourceGetCmd = &cobra.Command{
	Use: "get", Short: "Get data source settings", Args: cobra.NoArgs, RunE: pipelineGetSettings,
}
var pipelineDataSourceUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update data source settings", Args: cobra.NoArgs, RunE: pipelineUpdateSettings,
}
var pipelineSearchCmd = &cobra.Command{
	Use: "search", Short: "Manage recommendation search", Args: cobra.NoArgs,
}
var pipelineSearchGetCmd = &cobra.Command{
	Use: "get", Short: "Get search settings", Args: cobra.NoArgs, RunE: pipelineGetSettings,
}
var pipelineSearchUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update search settings", Args: cobra.NoArgs, RunE: pipelineUpdateSettings,
}
var pipelineRankerCmd = &cobra.Command{
	Use: "ranker", Short: "Manage recommendation ranking", Args: cobra.NoArgs,
}
var pipelineRankerGetCmd = &cobra.Command{
	Use: "get", Short: "Get ranker settings", Args: cobra.NoArgs, RunE: pipelineGetSettings,
}
var pipelineRankerUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update ranker settings", Args: cobra.NoArgs, RunE: pipelineUpdateSettings,
}
var pipelineFallbackCmd = &cobra.Command{
	Use: "fallback", Short: "Manage fallback recommendations", Args: cobra.NoArgs,
}
var pipelineFallbackGetCmd = &cobra.Command{
	Use: "get", Short: "Get fallback settings", Args: cobra.NoArgs, RunE: pipelineGetSettings,
}
var pipelineFallbackUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update fallback settings", Args: cobra.NoArgs, RunE: pipelineUpdateSettings,
}
var pipelineReplacementCmd = &cobra.Command{
	Use: "replacement", Short: "Manage replacement recommendations", Args: cobra.NoArgs,
}
var pipelineReplacementGetCmd = &cobra.Command{
	Use: "get", Short: "Get replacement settings", Args: cobra.NoArgs, RunE: pipelineGetSettings,
}
var pipelineReplacementUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update replacement settings", Args: cobra.NoArgs, RunE: pipelineUpdateSettings,
}

func pipelineGetSettings(cmd *cobra.Command, args []string) error {
	var p Pipeline
	if err := p.Load(newClient(cmd)); err != nil {
		return err
	}
	return marshal(cmd, p.rawRecommend[strings.ReplaceAll(cmd.Parent().Name(), "-", "_")])
}

// Only changed local flags contribute to the patch. Inherited connection flags
// must not turn a no-op update into a request.
func pipelineUpdateSettings(cmd *cobra.Command, args []string) error {
	changes := make(map[string]any)
	cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if !flag.Changed {
			return
		}
		path := strings.ReplaceAll(flag.Name, "-", "_")
		switch flag.Name {
		case "positive-feedback-type", "negative-feedback-type", "read-feedback-type":
			path += "s"
		case "column":
			path = "columns"
		case "recommender":
			path = "recommenders"
		case "early-stopping-patience":
			path = "early_stopping.patience"
		default:
			for _, provider := range []string{"reranker", "decision"} {
				if strings.HasPrefix(flag.Name, provider+"-") {
					path = provider + "_api." + strings.TrimPrefix(path, provider+"_")
				}
			}
		}
		var value any
		switch flag.Value.Type() {
		case "string":
			value, _ = cmd.Flags().GetString(flag.Name)
		case "int":
			value, _ = cmd.Flags().GetInt(flag.Name)
		case "uint":
			value, _ = cmd.Flags().GetUint(flag.Name)
		case "bool":
			value, _ = cmd.Flags().GetBool(flag.Name)
		case "float64":
			value, _ = cmd.Flags().GetFloat64(flag.Name)
		case "duration":
			duration, _ := cmd.Flags().GetDuration(flag.Name)
			value = duration.String()
		case "stringArray":
			values, _ := cmd.Flags().GetStringArray(flag.Name)
			// A single explicit empty value clears a list, rather than sending [""].
			if len(values) == 1 && values[0] == "" {
				values = []string{}
			}
			value = values
		}
		target := changes
		keys := strings.Split(path, ".")
		for _, key := range keys[:len(keys)-1] {
			nested, ok := target[key].(map[string]any)
			if !ok {
				nested = make(map[string]any)
				target[key] = nested
			}
			target = nested
		}
		target[keys[len(keys)-1]] = value
	})
	if len(changes) == 0 {
		return fmt.Errorf("no changes supplied")
	}
	if cmd.Parent() != PipelineCmd {
		changes = map[string]any{strings.ReplaceAll(cmd.Parent().Name(), "-", "_"): changes}
	}
	c := newClient(cmd)
	var p Pipeline
	if err := p.Load(c); err != nil {
		return err
	}
	if err := p.SaveChanges(c, changes); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Pipeline settings updated.")
	return nil
}

// Pipeline manages recommendation configuration. Writes are read-modify-write:
// the server has no compare-and-swap API; concurrent edits can overwrite changes.
// The Master validates the submitted configuration using its actual rules.
// Configuration fields and supporting types are copied from
// gorse/config/config.go at d3186be36756, without server-side validation tags.
// Keep fields and serialization tags aligned when updating the Gorse dependency.
type Pipeline struct {
	rawRecommend    map[string]any
	CacheSize       int                     `mapstructure:"cache_size"`
	CacheExpire     time.Duration           `mapstructure:"cache_expire"`
	ContextSize     int                     `mapstructure:"context_size"`
	ActiveUserTTL   int                     `mapstructure:"active_user_ttl"`
	DataSource      DataSourceConfig        `mapstructure:"data_source"`
	Search          SearchConfig            `mapstructure:"search"`
	NonPersonalized []NonPersonalizedConfig `mapstructure:"non-personalized"`
	ItemToItem      []ItemToItemConfig      `mapstructure:"item-to-item"`
	UserToUser      []UserToUserConfig      `mapstructure:"user-to-user"`
	Collaborative   CollaborativeConfig     `mapstructure:"collaborative"`
	External        []ExternalConfig        `mapstructure:"external"`
	Replacement     ReplacementConfig       `mapstructure:"replacement"`
	Ranker          RankerConfig            `mapstructure:"ranker"`
	Fallback        FallbackConfig          `mapstructure:"fallback"`
}

// Feedback expressions remain strings in their API representation; the server validates them.
type DataSourceConfig struct {
	PositiveFeedbackTypes []string `mapstructure:"positive_feedback_types"` // positive feedback type
	NegativeFeedbackTypes []string `mapstructure:"negative_feedback_types"` // negative feedback type (highest priority)
	ReadFeedbackTypes     []string `mapstructure:"read_feedback_types"`     // feedback type for read event
	PositiveFeedbackTTL   uint     `mapstructure:"positive_feedback_ttl"`   // time-to-live of positive feedbacks
	ItemTTL               uint     `mapstructure:"item_ttl"`                // item-to-live of items
}

type SearchConfig struct {
	Columns []string `mapstructure:"columns"`
}

type NonPersonalizedConfig struct {
	Name   string `mapstructure:"name" json:"name"`
	Score  string `mapstructure:"score" json:"score"`
	Filter string `mapstructure:"filter" json:"filter"`
}

type ItemToItemConfig struct {
	Name   string `mapstructure:"name" json:"name"`
	Type   string `mapstructure:"type" json:"type"`
	Column string `mapstructure:"column" json:"column"`
}

type UserToUserConfig struct {
	Name   string `mapstructure:"name" json:"name"`
	Type   string `mapstructure:"type" json:"type"`
	Column string `mapstructure:"column" json:"column"`
}

type CollaborativeConfig struct {
	Type           string              `mapstructure:"type"`
	FitPeriod      time.Duration       `mapstructure:"fit_period"`
	FitEpoch       int                 `mapstructure:"fit_epoch"`
	OptimizePeriod time.Duration       `mapstructure:"optimize_period"`
	OptimizeTrials int                 `mapstructure:"optimize_trials"`
	EarlyStopping  EarlyStoppingConfig `mapstructure:"early_stopping"`
}

type EarlyStoppingConfig struct {
	Patience int `mapstructure:"patience"`
}

type ExternalConfig struct {
	Name   string `mapstructure:"name" json:"name"`
	Script string `mapstructure:"script" json:"script"`
}

type ReplacementConfig struct {
	EnableReplacement        bool    `mapstructure:"enable_replacement"`
	PositiveReplacementDecay float64 `mapstructure:"positive_replacement_decay"`
	ReadReplacementDecay     float64 `mapstructure:"read_replacement_decay"`
}

type RankerConfig struct {
	Type           string              `mapstructure:"type"`
	Recommenders   []string            `mapstructure:"recommenders"`
	CacheExpire    time.Duration       `mapstructure:"cache_expire"`
	FitPeriod      time.Duration       `mapstructure:"fit_period"`
	FitEpoch       int                 `mapstructure:"fit_epoch"`
	OptimizePeriod time.Duration       `mapstructure:"optimize_period"`
	OptimizeTrials int                 `mapstructure:"optimize_trials"`
	EarlyStopping  EarlyStoppingConfig `mapstructure:"early_stopping"`
	RerankerAPI    RerankerAPIConfig   `mapstructure:"reranker_api"`
	DecisionAPI    DecisionAPIConfig   `mapstructure:"decision_api"`
}

type FallbackConfig struct {
	Recommenders []string `mapstructure:"recommenders"`
}

type DecisionAPIConfig struct {
	StateTemplate        string `mapstructure:"state_template"`
	InstructionsTemplate string `mapstructure:"instructions_template"`
	AuthToken            string `mapstructure:"auth_token" mask:"secret"`
	Model                string `mapstructure:"model"`
	URL                  string `mapstructure:"url"`
}

type RerankerAPIConfig struct {
	QueryTemplate    string `mapstructure:"query_template"`
	DocumentTemplate string `mapstructure:"document_template"`
	AuthToken        string `mapstructure:"auth_token" mask:"secret"`
	Model            string `mapstructure:"model"`
	URL              string `mapstructure:"url"`
}

func (r *Pipeline) ListRecommenders() []string {
	recommenders := make([]string, 0)
	for _, rec := range r.NonPersonalized {
		recommenders = append(recommenders, rec.FullName())
	}
	for _, rec := range r.ItemToItem {
		recommenders = append(recommenders, rec.FullName())
	}
	for _, rec := range r.UserToUser {
		recommenders = append(recommenders, rec.FullName())
	}
	for _, rec := range r.External {
		recommenders = append(recommenders, rec.FullName())
	}
	recommenders = append(recommenders, r.Collaborative.FullName())
	recommenders = append(recommenders, "latest")
	return recommenders
}

func (c *NonPersonalizedConfig) FullName() string {
	return "non-personalized/" + c.Name
}

func (c *ItemToItemConfig) FullName() string {
	return "item-to-item/" + c.Name
}

func (c *UserToUserConfig) FullName() string {
	return "user-to-user/" + c.Name
}

func (c *ExternalConfig) FullName() string {
	return "external/" + c.Name
}

func (c *CollaborativeConfig) FullName() string {
	return "collaborative"
}

// requireUnambiguousName protects operations that select one recommender by name.
// Other entries are left for the server to validate when saving configuration.
func (p *Pipeline) requireUnambiguousName(kind, name string) error {
	count := 0
	for _, fullName := range p.ListRecommenders() {
		if fullName == kind+"/"+name {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("duplicate %s name %q", kind, name)
	}
	return nil
}

// Load replaces the local fields with the current recommendation configuration
// from the client without applying business validation. This allows inspection
// and repair of existing configuration; the server validates writes.
func (p *Pipeline) Load(c *client.AdminClient) error {
	root, err := c.GetConfigMap()
	if err != nil {
		return err
	}
	_, config, ok := getConfigValue(root, "recommend", "Recommend")
	if !ok {
		return fmt.Errorf("recommend config not found")
	}
	raw, ok := config.(map[string]any)
	if !ok {
		return fmt.Errorf("recommend config must be an object")
	}
	if _, ok := raw["collaborative"]; !ok {
		return fmt.Errorf("collaborative config not found")
	}
	var value Pipeline
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     &value,
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
	})
	if err != nil {
		return err
	}
	if err := decoder.Decode(raw); err != nil {
		return err
	}
	value.rawRecommend = raw
	*p = value
	return nil
}

// SaveChanges overlays only requested fields onto the complete raw recommend
// object. Master replaces that object, rather than merging a nested patch.
// Retaining raw fields also preserves settings introduced by newer servers.
func (p *Pipeline) SaveChanges(c *client.AdminClient, changes map[string]any) error {
	mergePipelineChanges(p.rawRecommend, changes)
	_, err := c.UpdateConfig(map[string]any{"recommend": p.rawRecommend})
	return err
}

func mergePipelineChanges(target, changes map[string]any) {
	for key, value := range changes {
		if nested, ok := value.(map[string]any); ok {
			current, ok := target[key].(map[string]any)
			if !ok {
				current = make(map[string]any)
				target[key] = current
			}
			mergePipelineChanges(current, nested)
		} else {
			target[key] = value
		}
	}
}

// encodePipeline applies the upstream mapstructure tags and converts durations
// to strings before JSON encoding so nanosecond precision is retained.
func encodePipeline(value Pipeline) (map[string]any, error) {
	var fields map[string]any
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &fields, Deep: true})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(value); err != nil {
		return nil, err
	}
	data, err := json.Marshal(formatConfigMap(fields))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// Save submits the local recommendation configuration for server validation.
func (p *Pipeline) Save(c *client.AdminClient) error {
	value, err := encodePipeline(*p)
	if err != nil {
		return err
	}
	_, err = c.UpdateConfig(map[string]any{"recommend": value})
	return err
}

func init() {
	PipelineCmd.AddCommand(pipelineUpdateCmd, pipelineDataSourceCmd, pipelineSearchCmd,
		pipelineRankerCmd, pipelineFallbackCmd, pipelineReplacementCmd)
	pipelineDataSourceCmd.AddCommand(pipelineDataSourceGetCmd, pipelineDataSourceUpdateCmd)
	pipelineSearchCmd.AddCommand(pipelineSearchGetCmd, pipelineSearchUpdateCmd)
	pipelineRankerCmd.AddCommand(pipelineRankerGetCmd, pipelineRankerUpdateCmd)
	pipelineFallbackCmd.AddCommand(pipelineFallbackGetCmd, pipelineFallbackUpdateCmd)
	pipelineReplacementCmd.AddCommand(pipelineReplacementGetCmd, pipelineReplacementUpdateCmd)

	pipelineUpdateCmd.Flags().Int("cache-size", 0, "Recommendation cache size")
	pipelineUpdateCmd.Flags().Duration("cache-expire", 0, "Recommendation cache expiration (e.g. 24h)")
	pipelineUpdateCmd.Flags().Int("context-size", 0, "Recommendation context size")
	pipelineUpdateCmd.Flags().Int("active-user-ttl", 0, "Active user time-to-live in days (0 disables expiration)")

	pipelineDataSourceUpdateCmd.Flags().StringArray("positive-feedback-type", nil, "Positive feedback expression (repeat for each; empty clears the list)")
	pipelineDataSourceUpdateCmd.Flags().StringArray("negative-feedback-type", nil, "Negative feedback expression (repeat for each; empty clears the list)")
	pipelineDataSourceUpdateCmd.Flags().StringArray("read-feedback-type", nil, "Read feedback expression (repeat for each; empty clears the list)")
	pipelineDataSourceUpdateCmd.Flags().Uint("positive-feedback-ttl", 0, "Positive feedback time-to-live in days (0 disables expiration)")
	pipelineDataSourceUpdateCmd.Flags().Uint("item-ttl", 0, "Item time-to-live in days (0 disables expiration)")
	pipelineSearchUpdateCmd.Flags().StringArray("column", nil, "Search column expression (repeat for each; empty clears the list)")

	pipelineRankerUpdateCmd.Flags().String("type", "", "Ranker type: none, fm, reranker or decision")
	pipelineRankerUpdateCmd.Flags().StringArray("recommender", nil, "Ordered recommender reference (repeat for each; empty clears the list)")
	pipelineRankerUpdateCmd.Flags().Duration("cache-expire", 0, "Ranker cache expiration (e.g. 24h)")
	pipelineRankerUpdateCmd.Flags().Duration("fit-period", 0, "Model fitting period (e.g. 1h)")
	pipelineRankerUpdateCmd.Flags().Int("fit-epoch", 0, "Number of fitting epochs")
	pipelineRankerUpdateCmd.Flags().Duration("optimize-period", 0, "Model optimization period (0s disables optimization)")
	pipelineRankerUpdateCmd.Flags().Int("optimize-trials", 0, "Number of optimization trials")
	pipelineRankerUpdateCmd.Flags().Int("early-stopping-patience", 0, "Early stopping patience")
	for _, provider := range []string{"reranker", "decision"} {
		pipelineRankerUpdateCmd.Flags().String(provider+"-url", "", "Provider API URL")
		pipelineRankerUpdateCmd.Flags().String(provider+"-model", "", "Provider model")
		pipelineRankerUpdateCmd.Flags().String(provider+"-auth-token", "", "Provider authentication token")
	}
	pipelineRankerUpdateCmd.Flags().String("reranker-query-template", "", "Reranker query template")
	pipelineRankerUpdateCmd.Flags().String("reranker-document-template", "", "Reranker document template")
	pipelineRankerUpdateCmd.Flags().String("decision-state-template", "", "Decision state template")
	pipelineRankerUpdateCmd.Flags().String("decision-instructions-template", "", "Decision instructions template")

	pipelineFallbackUpdateCmd.Flags().StringArray("recommender", nil, "Ordered fallback reference (repeat for each; empty clears the list)")
	pipelineReplacementUpdateCmd.Flags().Bool("enable-replacement", false, "Enable replacement recommendations")
	pipelineReplacementUpdateCmd.Flags().Float64("positive-replacement-decay", 0, "Positive feedback replacement decay")
	pipelineReplacementUpdateCmd.Flags().Float64("read-replacement-decay", 0, "Read feedback replacement decay")

	PipelineCmd.AddCommand(
		pipelineGetCmd,
		pipelineResetCmd,
		pipelineItemToItemCmd,
		pipelineUserToUserCmd,
		pipelineNonPersonalizedCmd,
		pipelineExternalCmd,
		pipelineCollaborativeCmd,
	)
	pipelineItemToItemCmd.AddCommand(
		pipelineItemToItemListCmd,
		pipelineItemToItemGetCmd,
		pipelineItemToItemAddCmd,
		pipelineItemToItemUpdateCmd,
		pipelineItemToItemDeleteCmd,
	)
	pipelineUserToUserCmd.AddCommand(
		pipelineUserToUserListCmd,
		pipelineUserToUserGetCmd,
		pipelineUserToUserAddCmd,
		pipelineUserToUserUpdateCmd,
		pipelineUserToUserDeleteCmd,
	)
	pipelineNonPersonalizedCmd.AddCommand(
		pipelineNonPersonalizedListCmd,
		pipelineNonPersonalizedGetCmd,
		pipelineNonPersonalizedAddCmd,
		pipelineNonPersonalizedUpdateCmd,
		pipelineNonPersonalizedDeleteCmd,
	)
	pipelineExternalCmd.AddCommand(
		pipelineExternalListCmd,
		pipelineExternalGetCmd,
		pipelineExternalAddCmd,
		pipelineExternalUpdateCmd,
		pipelineExternalDeleteCmd,
	)
	pipelineCollaborativeCmd.AddCommand(
		pipelineCollaborativeGetCmd,
		pipelineCollaborativeUpdateCmd,
	)

	pipelineItemToItemAddCmd.Flags().String("type", "", "Recommender type")
	pipelineItemToItemAddCmd.Flags().String("column", "", "Recommender column")
	pipelineItemToItemUpdateCmd.Flags().String("type", "", "Recommender type")
	pipelineItemToItemUpdateCmd.Flags().String("column", "", "Recommender column")
	_ = pipelineItemToItemAddCmd.MarkFlagRequired("type")

	pipelineUserToUserAddCmd.Flags().String("type", "", "Recommender type")
	pipelineUserToUserAddCmd.Flags().String("column", "", "Recommender column")
	pipelineUserToUserUpdateCmd.Flags().String("type", "", "Recommender type")
	pipelineUserToUserUpdateCmd.Flags().String("column", "", "Recommender column")
	_ = pipelineUserToUserAddCmd.MarkFlagRequired("type")

	pipelineNonPersonalizedAddCmd.Flags().String("score", "", "Recommender score")
	pipelineNonPersonalizedAddCmd.Flags().String("filter", "", "Recommender filter")
	pipelineNonPersonalizedUpdateCmd.Flags().String("score", "", "Recommender score")
	pipelineNonPersonalizedUpdateCmd.Flags().String("filter", "", "Recommender filter")
	_ = pipelineNonPersonalizedAddCmd.MarkFlagRequired("score")

	pipelineExternalAddCmd.Flags().String("script", "", "Recommender script")
	pipelineExternalAddCmd.Flags().String("script-file", "", "Recommender script-file")
	pipelineExternalUpdateCmd.Flags().String("script", "", "Recommender script")
	pipelineExternalUpdateCmd.Flags().String("script-file", "", "Recommender script-file")
	pipelineExternalAddCmd.MarkFlagsMutuallyExclusive("script", "script-file")
	pipelineExternalAddCmd.MarkFlagsOneRequired("script", "script-file")
	pipelineExternalUpdateCmd.MarkFlagsMutuallyExclusive("script", "script-file")

	pipelineCollaborativeUpdateCmd.Flags().String("type", "", "Collaborative filtering type: none or mf")
	pipelineCollaborativeUpdateCmd.Flags().Duration("fit-period", 0, "Model fitting period (e.g. 1h)")
	pipelineCollaborativeUpdateCmd.Flags().Int("fit-epoch", 0, "Number of fitting epochs")
	pipelineCollaborativeUpdateCmd.Flags().Duration("optimize-period", 0, "Model optimization period (0s disables optimization)")
	pipelineCollaborativeUpdateCmd.Flags().Int("optimize-trials", 0, "Number of optimization trials")
}
