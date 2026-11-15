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
	"reflect"
	"slices"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/go-viper/mapstructure/v2"
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
		value, err := NewPipeline(newClient(cmd)).Get()
		if err != nil {
			return err
		}
		return marshal(cmd, formatConfigMap(value))
	},
}
var pipelineSchemaCmd = &cobra.Command{
	Use: "schema", Short: "Get recommendation pipeline configuration schema", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).Schema()
		if err != nil {
			return err
		}
		return marshal(cmd, value)
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

var pipelineItemToItemCmd = &cobra.Command{
	Use:   "item-to-item",
	Short: "Manage item-to-item recommenders",
	Long:  "Manage item-to-item recommenders. Adding a recommender does not change explicit ranker or fallback references. Deletion is rejected while such references exist.",
	Args:  cobra.NoArgs,
}

var pipelineItemToItemListCmd = &cobra.Command{
	Use:   "list",
	Short: "list item-to-item recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).ListItemToItem()
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineItemToItemGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get item-to-item recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).GetItemToItem(args[0])
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineItemToItemAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add item-to-item recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		typ, _ := cmd.Flags().GetString("type")
		column, _ := cmd.Flags().GetString("column")
		value := ItemToItemConfig{Name: args[0], Type: typ, Column: column}
		if err := NewPipeline(newClient(cmd)).AddItemToItem(value); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", args[0])
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
		if err := NewPipeline(newClient(cmd)).UpdateItemToItem(args[0], ItemToItemPatch{Type: pipelineStringPatch(cmd, "type"), Column: pipelineStringPatch(cmd, "column")}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", args[0])
		return nil
	},
}

var pipelineItemToItemDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete item-to-item recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := NewPipeline(newClient(cmd)).DeleteItemToItem(args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", args[0])
		return nil
	},
}

var pipelineUserToUserCmd = &cobra.Command{
	Use:   "user-to-user",
	Short: "Manage user-to-user recommenders",
	Long:  "Manage user-to-user recommenders. Adding a recommender does not change explicit ranker or fallback references. Deletion is rejected while such references exist.",
	Args:  cobra.NoArgs,
}

var pipelineUserToUserListCmd = &cobra.Command{
	Use:   "list",
	Short: "list user-to-user recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).ListUserToUser()
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineUserToUserGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get user-to-user recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).GetUserToUser(args[0])
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineUserToUserAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add user-to-user recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		typ, _ := cmd.Flags().GetString("type")
		column, _ := cmd.Flags().GetString("column")
		value := UserToUserConfig{Name: args[0], Type: typ, Column: column}
		if err := NewPipeline(newClient(cmd)).AddUserToUser(value); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", args[0])
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
		if err := NewPipeline(newClient(cmd)).UpdateUserToUser(args[0], UserToUserPatch{Type: pipelineStringPatch(cmd, "type"), Column: pipelineStringPatch(cmd, "column")}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", args[0])
		return nil
	},
}

var pipelineUserToUserDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete user-to-user recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := NewPipeline(newClient(cmd)).DeleteUserToUser(args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", args[0])
		return nil
	},
}

var pipelineNonPersonalizedCmd = &cobra.Command{
	Use:   "non-personalized",
	Short: "Manage non-personalized recommenders",
	Long:  "Manage non-personalized recommenders. Adding a recommender does not change explicit ranker or fallback references. Deletion is rejected while such references exist.",
	Args:  cobra.NoArgs,
}

var pipelineNonPersonalizedListCmd = &cobra.Command{
	Use:   "list",
	Short: "list non-personalized recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).ListNonPersonalized()
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineNonPersonalizedGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get non-personalized recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).GetNonPersonalized(args[0])
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineNonPersonalizedAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add non-personalized recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		score, _ := cmd.Flags().GetString("score")
		filter, _ := cmd.Flags().GetString("filter")
		value := NonPersonalizedConfig{Name: args[0], Score: score, Filter: filter}
		if err := NewPipeline(newClient(cmd)).AddNonPersonalized(value); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", args[0])
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
		if err := NewPipeline(newClient(cmd)).UpdateNonPersonalized(args[0], NonPersonalizedPatch{Score: pipelineStringPatch(cmd, "score"), Filter: pipelineStringPatch(cmd, "filter")}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", args[0])
		return nil
	},
}

var pipelineNonPersonalizedDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete non-personalized recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := NewPipeline(newClient(cmd)).DeleteNonPersonalized(args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", args[0])
		return nil
	},
}

var pipelineExternalCmd = &cobra.Command{
	Use:   "external",
	Short: "Manage external recommenders",
	Long:  "Manage external recommenders. Adding a recommender does not change explicit ranker or fallback references. Deletion is rejected while such references exist.",
	Args:  cobra.NoArgs,
}

var pipelineExternalListCmd = &cobra.Command{
	Use:   "list",
	Short: "list external recommender configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).ListExternal()
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineExternalGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "get external recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).GetExternal(args[0])
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineExternalAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "add external recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		script, err := pipelineScript(cmd)
		if err != nil {
			return err
		}
		value := ExternalConfig{Name: args[0]}
		if script != nil {
			value.Script = *script
		}
		if err := NewPipeline(newClient(cmd)).AddExternal(value); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "add succeeded:", args[0])
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
		script, err := pipelineScript(cmd)
		if err != nil {
			return err
		}
		if err := NewPipeline(newClient(cmd)).UpdateExternal(args[0], ExternalPatch{Script: script}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "update succeeded:", args[0])
		return nil
	},
}

var pipelineExternalDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "delete external recommender configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := NewPipeline(newClient(cmd)).DeleteExternal(args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "delete succeeded:", args[0])
		return nil
	},
}

var pipelineCollaborativeCmd = &cobra.Command{
	Use: "collaborative", Short: "Manage collaborative filtering", Args: cobra.NoArgs,
}

var pipelineCollaborativeGetCmd = &cobra.Command{
	Use: "get", Short: "Get collaborative filtering configuration", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		value, err := NewPipeline(newClient(cmd)).GetCollaborative()
		if err != nil {
			return err
		}
		return marshal(cmd, value)
	},
}

var pipelineCollaborativeUpdateCmd = &cobra.Command{
	Use: "update", Short: "Update collaborative filtering configuration", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	},
}

// PipelineTransport is the configuration surface used by Pipeline.
// *client.AdminClient implements it.
type PipelineTransport interface {
	GetConfigMap() (map[string]any, error)
	UpdateConfig(map[string]any) (map[string]any, error)
	GetConfigSchema() (map[string]any, error)
	ResetConfig() (map[string]any, error)
}

// Pipeline manages recommendation configuration. Writes are read-modify-write:
// the server has no compare-and-swap API; concurrent edits can overwrite changes.
// Raw maps are retained internally so unrecognized fields survive updates.
// The Master validates the submitted configuration using its actual rules.
// Configuration fields and supporting types are copied from
// gorse/config/config.go at d3186be36756. Keep fields and tags aligned when
// updating the Gorse dependency.
type Pipeline struct {
	client PipelineTransport `mapstructure:"-"`

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

func NewPipeline(client PipelineTransport) *Pipeline { return &Pipeline{client: client} }

func (p *Pipeline) read() (map[string]any, string, map[string]any, error) {
	root, err := p.client.GetConfigMap()
	if err != nil {
		return nil, "", nil, err
	}
	key, value, ok := getConfigValue(root, "recommend", "Recommend")
	if !ok {
		return nil, "", nil, fmt.Errorf("recommend config not found")
	}
	rec, ok := value.(map[string]any)
	if !ok {
		return nil, "", nil, fmt.Errorf("recommend config must be an object")
	}
	return root, key, rec, nil
}
func (p *Pipeline) Get() (map[string]any, error)    { _, _, rec, err := p.read(); return rec, err }
func (p *Pipeline) Schema() (map[string]any, error) { return p.client.GetConfigSchema() }
func (p *Pipeline) Reset() (map[string]any, error)  { return p.client.ResetConfig() }
func (p *Pipeline) Patch(document []byte) (map[string]any, error) {
	root, key, rec, err := p.read()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.DecodePatch(document)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON patch %w", err)
	}
	data, err = patch.Apply(data)
	if err != nil {
		return nil, fmt.Errorf("failed to apply JSON patch %w", err)
	}
	var updated map[string]any
	if err = json.Unmarshal(data, &updated); err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("recommend config must be an object")
	}
	root[key] = updated
	return p.client.UpdateConfig(root)
}

// pipelineState keeps the API representation at the transport boundary while
// recommendation operations use the local configuration types.
type pipelineState struct {
	Pipeline
	root     map[string]any
	raw      map[string]any
	original map[string]any
}

func (p *Pipeline) readPipeline() (*pipelineState, error) {
	root, _, raw, err := p.read()
	if err != nil {
		return nil, err
	}
	state := &pipelineState{root: root, raw: raw}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     &state.Pipeline,
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(raw); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, fullName := range state.ListRecommenders() {
		kind, name, named := strings.Cut(fullName, "/")
		if !named {
			continue
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("recommend.%s: name must not be empty", kind)
		}
		if seen[fullName] {
			return nil, fmt.Errorf("duplicate %s name %q", kind, name)
		}
		seen[fullName] = true
	}
	state.original, err = encodePipeline(state.Pipeline)
	if err != nil {
		return nil, err
	}
	return state, nil
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

func (p *Pipeline) writePipeline(state *pipelineState) error {
	updated, err := encodePipeline(state.Pipeline)
	if err != nil {
		return err
	}
	for field, value := range updated {
		if !reflect.DeepEqual(state.original[field], value) {
			state.raw[field] = mergePipelineValue(state.raw[field], value)
		}
	}
	_, err = p.client.UpdateConfig(state.root)
	return err
}

// Preserve unknown API fields, matching recommender entries by name when a
// typed operation adds, updates, or removes an entry.
func mergePipelineValue(original, updated any) any {
	switch value := updated.(type) {
	case map[string]any:
		previous, ok := original.(map[string]any)
		if !ok || previous == nil {
			return value
		}
		for key, field := range value {
			previous[key] = mergePipelineValue(previous[key], field)
		}
		return previous
	case []any:
		previous, _ := original.([]any)
		byName := make(map[string]map[string]any, len(previous))
		for _, entry := range previous {
			if fields, ok := entry.(map[string]any); ok {
				if name, ok := fields["name"].(string); ok {
					byName[name] = fields
				}
			}
		}
		for i, entry := range value {
			if fields, ok := entry.(map[string]any); ok {
				if name, ok := fields["name"].(string); ok {
					value[i] = mergePipelineValue(byName[name], fields)
				}
			}
		}
		return value
	default:
		return value
	}
}

func checkPipelineReferences(recommend *Pipeline, kind, name string) error {
	fullName := kind + "/" + name
	for i, ref := range recommend.Ranker.Recommenders {
		if ref == fullName {
			return fmt.Errorf("%s %q is referenced by recommend.ranker.recommenders[%d]", kind, name, i)
		}
	}
	for i, ref := range recommend.Fallback.Recommenders {
		if ref == fullName {
			return fmt.Errorf("%s %q is referenced by recommend.fallback.recommenders[%d]", kind, name, i)
		}
	}
	return nil
}

// ItemToItemPatch distinguishes omitted fields from explicit zero values.
type ItemToItemPatch struct {
	Type   *string `json:"type,omitempty"`
	Column *string `json:"column,omitempty"`
}

func (p *Pipeline) ListItemToItem() ([]ItemToItemConfig, error) {
	state, err := p.readPipeline()
	if err != nil {
		return nil, err
	}
	return append([]ItemToItemConfig{}, state.ItemToItem...), nil
}

func (p *Pipeline) GetItemToItem(name string) (ItemToItemConfig, error) {
	if strings.TrimSpace(name) == "" {
		return ItemToItemConfig{}, fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return ItemToItemConfig{}, err
	}
	for _, value := range state.ItemToItem {
		if value.Name == name {
			return value, nil
		}
	}
	return ItemToItemConfig{}, fmt.Errorf("item-to-item %q not found", name)
}

func (p *Pipeline) AddItemToItem(value ItemToItemConfig) error {
	if strings.TrimSpace(value.Name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for _, current := range state.ItemToItem {
		if current.Name == value.Name {
			return fmt.Errorf("item-to-item %q already exists", value.Name)
		}
	}
	state.ItemToItem = append(state.ItemToItem, value)
	return p.writePipeline(state)
}
func (p *Pipeline) UpdateItemToItem(name string, patch ItemToItemPatch) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	if patch.Type == nil && patch.Column == nil {
		return fmt.Errorf("no changes supplied")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i := range state.ItemToItem {
		if state.ItemToItem[i].Name == name {
			if patch.Type != nil {
				state.ItemToItem[i].Type = *patch.Type
			}
			if patch.Column != nil {
				state.ItemToItem[i].Column = *patch.Column
			}
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("item-to-item %q not found", name)
}

func (p *Pipeline) DeleteItemToItem(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i, value := range state.ItemToItem {
		if value.Name == name {
			if err := checkPipelineReferences(&state.Pipeline, "item-to-item", name); err != nil {
				return err
			}
			state.ItemToItem = slices.Delete(state.ItemToItem, i, i+1)
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("item-to-item %q not found", name)
}

// UserToUserPatch distinguishes omitted fields from explicit zero values.
type UserToUserPatch struct {
	Type   *string `json:"type,omitempty"`
	Column *string `json:"column,omitempty"`
}

func (p *Pipeline) ListUserToUser() ([]UserToUserConfig, error) {
	state, err := p.readPipeline()
	if err != nil {
		return nil, err
	}
	return append([]UserToUserConfig{}, state.UserToUser...), nil
}

func (p *Pipeline) GetUserToUser(name string) (UserToUserConfig, error) {
	if strings.TrimSpace(name) == "" {
		return UserToUserConfig{}, fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return UserToUserConfig{}, err
	}
	for _, value := range state.UserToUser {
		if value.Name == name {
			return value, nil
		}
	}
	return UserToUserConfig{}, fmt.Errorf("user-to-user %q not found", name)
}

func (p *Pipeline) AddUserToUser(value UserToUserConfig) error {
	if strings.TrimSpace(value.Name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for _, current := range state.UserToUser {
		if current.Name == value.Name {
			return fmt.Errorf("user-to-user %q already exists", value.Name)
		}
	}
	state.UserToUser = append(state.UserToUser, value)
	return p.writePipeline(state)
}
func (p *Pipeline) UpdateUserToUser(name string, patch UserToUserPatch) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	if patch.Type == nil && patch.Column == nil {
		return fmt.Errorf("no changes supplied")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i := range state.UserToUser {
		if state.UserToUser[i].Name == name {
			if patch.Type != nil {
				state.UserToUser[i].Type = *patch.Type
			}
			if patch.Column != nil {
				state.UserToUser[i].Column = *patch.Column
			}
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("user-to-user %q not found", name)
}

func (p *Pipeline) DeleteUserToUser(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i, value := range state.UserToUser {
		if value.Name == name {
			if err := checkPipelineReferences(&state.Pipeline, "user-to-user", name); err != nil {
				return err
			}
			state.UserToUser = slices.Delete(state.UserToUser, i, i+1)
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("user-to-user %q not found", name)
}

// NonPersonalizedPatch distinguishes omitted fields from explicit zero values.
type NonPersonalizedPatch struct {
	Score  *string `json:"score,omitempty"`
	Filter *string `json:"filter,omitempty"`
}

func (p *Pipeline) ListNonPersonalized() ([]NonPersonalizedConfig, error) {
	state, err := p.readPipeline()
	if err != nil {
		return nil, err
	}
	return append([]NonPersonalizedConfig{}, state.NonPersonalized...), nil
}

func (p *Pipeline) GetNonPersonalized(name string) (NonPersonalizedConfig, error) {
	if strings.TrimSpace(name) == "" {
		return NonPersonalizedConfig{}, fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return NonPersonalizedConfig{}, err
	}
	for _, value := range state.NonPersonalized {
		if value.Name == name {
			return value, nil
		}
	}
	return NonPersonalizedConfig{}, fmt.Errorf("non-personalized %q not found", name)
}

func (p *Pipeline) AddNonPersonalized(value NonPersonalizedConfig) error {
	if strings.TrimSpace(value.Name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for _, current := range state.NonPersonalized {
		if current.Name == value.Name {
			return fmt.Errorf("non-personalized %q already exists", value.Name)
		}
	}
	state.NonPersonalized = append(state.NonPersonalized, value)
	return p.writePipeline(state)
}
func (p *Pipeline) UpdateNonPersonalized(name string, patch NonPersonalizedPatch) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	if patch.Score == nil && patch.Filter == nil {
		return fmt.Errorf("no changes supplied")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i := range state.NonPersonalized {
		if state.NonPersonalized[i].Name == name {
			if patch.Score != nil {
				state.NonPersonalized[i].Score = *patch.Score
			}
			if patch.Filter != nil {
				state.NonPersonalized[i].Filter = *patch.Filter
			}
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("non-personalized %q not found", name)
}

func (p *Pipeline) DeleteNonPersonalized(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i, value := range state.NonPersonalized {
		if value.Name == name {
			if err := checkPipelineReferences(&state.Pipeline, "non-personalized", name); err != nil {
				return err
			}
			state.NonPersonalized = slices.Delete(state.NonPersonalized, i, i+1)
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("non-personalized %q not found", name)
}

// ExternalPatch distinguishes omitted fields from explicit zero values.
type ExternalPatch struct {
	Script *string `json:"script,omitempty"`
}

func (p *Pipeline) ListExternal() ([]ExternalConfig, error) {
	state, err := p.readPipeline()
	if err != nil {
		return nil, err
	}
	return append([]ExternalConfig{}, state.External...), nil
}

func (p *Pipeline) GetExternal(name string) (ExternalConfig, error) {
	if strings.TrimSpace(name) == "" {
		return ExternalConfig{}, fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return ExternalConfig{}, err
	}
	for _, value := range state.External {
		if value.Name == name {
			return value, nil
		}
	}
	return ExternalConfig{}, fmt.Errorf("external %q not found", name)
}

func (p *Pipeline) AddExternal(value ExternalConfig) error {
	if strings.TrimSpace(value.Name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for _, current := range state.External {
		if current.Name == value.Name {
			return fmt.Errorf("external %q already exists", value.Name)
		}
	}
	state.External = append(state.External, value)
	return p.writePipeline(state)
}
func (p *Pipeline) UpdateExternal(name string, patch ExternalPatch) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	if patch.Script == nil {
		return fmt.Errorf("no changes supplied")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i := range state.External {
		if state.External[i].Name == name {
			if patch.Script != nil {
				state.External[i].Script = *patch.Script
			}
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("external %q not found", name)
}

func (p *Pipeline) DeleteExternal(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	for i, value := range state.External {
		if value.Name == name {
			if err := checkPipelineReferences(&state.Pipeline, "external", name); err != nil {
				return err
			}
			state.External = slices.Delete(state.External, i, i+1)
			return p.writePipeline(state)
		}
	}
	return fmt.Errorf("external %q not found", name)
}

// CollaborativePatch updates the singleton collaborative configuration.
type CollaborativePatch struct {
	Type           *string        `json:"type,omitempty"`
	FitPeriod      *time.Duration `json:"fit_period,omitempty"`
	FitEpoch       *int           `json:"fit_epoch,omitempty"`
	OptimizePeriod *time.Duration `json:"optimize_period,omitempty"`
	OptimizeTrials *int           `json:"optimize_trials,omitempty"`
}

func (p *Pipeline) GetCollaborative() (CollaborativeConfig, error) {
	state, err := p.readPipeline()
	if err != nil {
		return CollaborativeConfig{}, err
	}
	if _, ok := state.raw["collaborative"]; !ok {
		return CollaborativeConfig{}, fmt.Errorf("collaborative config not found")
	}
	return state.Collaborative, nil
}

func (p *Pipeline) UpdateCollaborative(patch CollaborativePatch) error {
	if patch.Type == nil && patch.FitPeriod == nil && patch.FitEpoch == nil && patch.OptimizePeriod == nil && patch.OptimizeTrials == nil {
		return fmt.Errorf("no changes supplied")
	}
	state, err := p.readPipeline()
	if err != nil {
		return err
	}
	if _, ok := state.raw["collaborative"]; !ok {
		return fmt.Errorf("collaborative config not found")
	}
	if patch.Type != nil {
		state.Collaborative.Type = *patch.Type
	}
	if patch.FitPeriod != nil {
		state.Collaborative.FitPeriod = *patch.FitPeriod
	}
	if patch.FitEpoch != nil {
		state.Collaborative.FitEpoch = *patch.FitEpoch
	}
	if patch.OptimizePeriod != nil {
		state.Collaborative.OptimizePeriod = *patch.OptimizePeriod
	}
	if patch.OptimizeTrials != nil {
		state.Collaborative.OptimizeTrials = *patch.OptimizeTrials
	}
	return p.writePipeline(state)
}

func init() {
	PipelineCmd.AddCommand(pipelineGetCmd, pipelineSchemaCmd, pipelinePatchCmd, pipelineResetCmd)
	PipelineCmd.AddCommand(pipelineItemToItemCmd, pipelineUserToUserCmd, pipelineNonPersonalizedCmd, pipelineExternalCmd, pipelineCollaborativeCmd)
	pipelineItemToItemCmd.AddCommand(pipelineItemToItemListCmd, pipelineItemToItemGetCmd, pipelineItemToItemAddCmd, pipelineItemToItemUpdateCmd, pipelineItemToItemDeleteCmd)
	pipelineUserToUserCmd.AddCommand(pipelineUserToUserListCmd, pipelineUserToUserGetCmd, pipelineUserToUserAddCmd, pipelineUserToUserUpdateCmd, pipelineUserToUserDeleteCmd)
	pipelineNonPersonalizedCmd.AddCommand(pipelineNonPersonalizedListCmd, pipelineNonPersonalizedGetCmd, pipelineNonPersonalizedAddCmd, pipelineNonPersonalizedUpdateCmd, pipelineNonPersonalizedDeleteCmd)
	pipelineExternalCmd.AddCommand(pipelineExternalListCmd, pipelineExternalGetCmd, pipelineExternalAddCmd, pipelineExternalUpdateCmd, pipelineExternalDeleteCmd)
	pipelineCollaborativeCmd.AddCommand(pipelineCollaborativeGetCmd, pipelineCollaborativeUpdateCmd)

	for _, cmd := range []*cobra.Command{
		pipelineGetCmd,
		pipelineSchemaCmd,
		pipelinePatchCmd,
		pipelineResetCmd,
		pipelineItemToItemListCmd,
		pipelineItemToItemGetCmd,
		pipelineItemToItemAddCmd,
		pipelineItemToItemUpdateCmd,
		pipelineItemToItemDeleteCmd,
		pipelineUserToUserListCmd,
		pipelineUserToUserGetCmd,
		pipelineUserToUserAddCmd,
		pipelineUserToUserUpdateCmd,
		pipelineUserToUserDeleteCmd,
		pipelineNonPersonalizedListCmd,
		pipelineNonPersonalizedGetCmd,
		pipelineNonPersonalizedAddCmd,
		pipelineNonPersonalizedUpdateCmd,
		pipelineNonPersonalizedDeleteCmd,
		pipelineExternalListCmd,
		pipelineExternalGetCmd,
		pipelineExternalAddCmd,
		pipelineExternalUpdateCmd,
		pipelineExternalDeleteCmd,
		pipelineCollaborativeGetCmd,
		pipelineCollaborativeUpdateCmd,
	} {
		cmd.Flags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
		cmd.Flags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
		cmd.Flags().String("application", "", "Gorse CLI application name")
	}

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
