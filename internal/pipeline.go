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

// Pipeline manages recommendation configuration. Writes are read-modify-write:
// the server has no compare-and-swap API; concurrent edits can overwrite changes.
// The Master validates the submitted configuration using its actual rules.
// Configuration fields and supporting types are copied from
// gorse/config/config.go at d3186be36756. Keep fields and tags aligned when
// updating the Gorse dependency.
type Pipeline struct {
	CacheSize       int                     `mapstructure:"cache_size" validate:"gt=0"`
	CacheExpire     time.Duration           `mapstructure:"cache_expire" validate:"gt=0"`
	ContextSize     int                     `mapstructure:"context_size" validate:"gt=0"`
	ActiveUserTTL   int                     `mapstructure:"active_user_ttl" validate:"gte=0"`
	DataSource      DataSourceConfig        `mapstructure:"data_source"`
	Search          SearchConfig            `mapstructure:"search"`
	NonPersonalized []NonPersonalizedConfig `mapstructure:"non-personalized" validate:"dive"`
	ItemToItem      []ItemToItemConfig      `mapstructure:"item-to-item" validate:"dive"`
	UserToUser      []UserToUserConfig      `mapstructure:"user-to-user" validate:"dive"`
	Collaborative   CollaborativeConfig     `mapstructure:"collaborative"`
	External        []ExternalConfig        `mapstructure:"external" validate:"dive"`
	Replacement     ReplacementConfig       `mapstructure:"replacement"`
	Ranker          RankerConfig            `mapstructure:"ranker"`
	Fallback        FallbackConfig          `mapstructure:"fallback"`
}

// Feedback expressions remain strings in their API representation; the server validates them.
type DataSourceConfig struct {
	PositiveFeedbackTypes []string `mapstructure:"positive_feedback_types"`                // positive feedback type
	NegativeFeedbackTypes []string `mapstructure:"negative_feedback_types"`                // negative feedback type (highest priority)
	ReadFeedbackTypes     []string `mapstructure:"read_feedback_types"`                    // feedback type for read event
	PositiveFeedbackTTL   uint     `mapstructure:"positive_feedback_ttl" validate:"gte=0"` // time-to-live of positive feedbacks
	ItemTTL               uint     `mapstructure:"item_ttl" validate:"gte=0"`              // item-to-live of items
}

type SearchConfig struct {
	Columns []string `mapstructure:"columns" validate:"dive,item_expr"`
}

type NonPersonalizedConfig struct {
	Name   string `mapstructure:"name" json:"name"`
	Score  string `mapstructure:"score" json:"score" validate:"required,item_expr"`
	Filter string `mapstructure:"filter" json:"filter" validate:"item_expr"`
}

type ItemToItemConfig struct {
	Name   string `mapstructure:"name" json:"name"`
	Type   string `mapstructure:"type" json:"type" validate:"oneof=embedding tags users auto"`
	Column string `mapstructure:"column" json:"column" validate:"item_expr"`
}

type UserToUserConfig struct {
	Name   string `mapstructure:"name" json:"name"`
	Type   string `mapstructure:"type" json:"type" validate:"oneof=embedding tags items auto"`
	Column string `mapstructure:"column" json:"column" validate:"item_expr"`
}

type CollaborativeConfig struct {
	Type           string              `mapstructure:"type" validate:"oneof=none mf"`
	FitPeriod      time.Duration       `mapstructure:"fit_period" validate:"gt=0"`
	FitEpoch       int                 `mapstructure:"fit_epoch" validate:"gt=0"`
	OptimizePeriod time.Duration       `mapstructure:"optimize_period" validate:"gte=0"`
	OptimizeTrials int                 `mapstructure:"optimize_trials" validate:"gt=0"`
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
	PositiveReplacementDecay float64 `mapstructure:"positive_replacement_decay" validate:"gt=0"`
	ReadReplacementDecay     float64 `mapstructure:"read_replacement_decay" validate:"gt=0"`
}

type RankerConfig struct {
	Type           string              `mapstructure:"type" validate:"oneof=none fm reranker decision"`
	Recommenders   []string            `mapstructure:"recommenders"`
	CacheExpire    time.Duration       `mapstructure:"cache_expire" validate:"gt=0"`
	FitPeriod      time.Duration       `mapstructure:"fit_period" validate:"gt=0"`
	FitEpoch       int                 `mapstructure:"fit_epoch" validate:"gt=0"`
	OptimizePeriod time.Duration       `mapstructure:"optimize_period" validate:"gte=0"`
	OptimizeTrials int                 `mapstructure:"optimize_trials" validate:"gt=0"`
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

// Load replaces the local fields with the current recommendation configuration
// from the client.
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
	seen := make(map[string]bool)
	for _, fullName := range value.ListRecommenders() {
		kind, name, named := strings.Cut(fullName, "/")
		if !named {
			continue
		}
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("recommend.%s: name must not be empty", kind)
		}
		if seen[fullName] {
			return fmt.Errorf("duplicate %s name %q", kind, name)
		}
		seen[fullName] = true
	}
	*p = value
	return nil
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
