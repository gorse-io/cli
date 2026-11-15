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
	"time"

	"github.com/evanphx/json-patch/v5"
	"github.com/go-viper/mapstructure/v2"
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
type Pipeline struct{ client PipelineTransport }

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

func decodePipeline(value any, target any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: target, DecodeHook: mapstructure.StringToTimeDurationHookFunc()})
	if err != nil {
		return err
	}
	return decoder.Decode(value)
}
func pipelineEntries(rec map[string]any, kind string) ([]any, error) {
	value := rec[kind]
	if value == nil {
		return []any{}, nil
	}
	entries, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("recommend.%s must be a list", kind)
	}
	seen := map[string]bool{}
	for i, entry := range entries {
		row, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("recommend.%s[%d] must be an object", kind, i)
		}
		name, ok := row["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("recommend.%s[%d]: name must not be empty", kind, i)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate %s name %q", kind, name)
		}
		seen[name] = true
	}
	return entries, nil
}
func (p *Pipeline) list(kind string, target any) error {
	_, _, rec, err := p.read()
	if err != nil {
		return err
	}
	entries, err := pipelineEntries(rec, kind)
	if err != nil {
		return err
	}
	return decodePipeline(entries, target)
}
func (p *Pipeline) get(kind, name string, target any) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	_, _, rec, err := p.read()
	if err != nil {
		return err
	}
	entries, err := pipelineEntries(rec, kind)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.(map[string]any)["name"] == name {
			return decodePipeline(entry, target)
		}
	}
	return fmt.Errorf("%s %q not found", kind, name)
}
func (p *Pipeline) mutate(kind, name, operation string, value any) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	var fields map[string]any
	if value != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &fields); err != nil {
			return err
		}
		if operation == "update" && len(fields) == 0 {
			return fmt.Errorf("no changes supplied")
		}
	}
	root, _, rec, err := p.read()
	if err != nil {
		return err
	}
	entries, err := pipelineEntries(rec, kind)
	if err != nil {
		return err
	}
	index := -1
	for i, entry := range entries {
		if entry.(map[string]any)["name"] == name {
			index = i
			break
		}
	}
	switch operation {
	case "add":
		if index >= 0 {
			return fmt.Errorf("%s %q already exists", kind, name)
		}
		entries = append(entries, fields)
	case "update":
		if index < 0 {
			return fmt.Errorf("%s %q not found", kind, name)
		}
		for key, v := range fields {
			entries[index].(map[string]any)[key] = v
		}
	case "delete":
		if index < 0 {
			return fmt.Errorf("%s %q not found", kind, name)
		}
		for _, section := range []string{"ranker", "fallback"} {
			var refs struct{ Recommenders []string }
			if err = decodePipeline(rec[section], &refs); err != nil {
				return err
			}
			for i, ref := range refs.Recommenders {
				if ref == kind+"/"+name {
					return fmt.Errorf("%s %q is referenced by recommend.%s.recommenders[%d]", kind, name, section, i)
				}
			}
		}
		entries = append(entries[:index], entries[index+1:]...)
	}
	rec[kind] = entries
	_, err = p.client.UpdateConfig(root)
	return err
}

// ItemToItemPatch distinguishes omitted fields from explicit zero values.
type ItemToItemPatch struct {
	Type   *string `json:"type,omitempty"`
	Column *string `json:"column,omitempty"`
}

func (p *Pipeline) ListItemToItem() ([]config.ItemToItemConfig, error) {
	values := make([]config.ItemToItemConfig, 0)
	err := p.list("item-to-item", &values)
	return values, err
}
func (p *Pipeline) GetItemToItem(name string) (config.ItemToItemConfig, error) {
	var value config.ItemToItemConfig
	err := p.get("item-to-item", name, &value)
	return value, err
}
func (p *Pipeline) AddItemToItem(value config.ItemToItemConfig) error {
	return p.mutate("item-to-item", value.Name, "add", value)
}
func (p *Pipeline) UpdateItemToItem(name string, patch ItemToItemPatch) error {
	return p.mutate("item-to-item", name, "update", patch)
}
func (p *Pipeline) DeleteItemToItem(name string) error {
	return p.mutate("item-to-item", name, "delete", nil)
}

// UserToUserPatch distinguishes omitted fields from explicit zero values.
type UserToUserPatch struct {
	Type   *string `json:"type,omitempty"`
	Column *string `json:"column,omitempty"`
}

func (p *Pipeline) ListUserToUser() ([]config.UserToUserConfig, error) {
	values := make([]config.UserToUserConfig, 0)
	err := p.list("user-to-user", &values)
	return values, err
}
func (p *Pipeline) GetUserToUser(name string) (config.UserToUserConfig, error) {
	var value config.UserToUserConfig
	err := p.get("user-to-user", name, &value)
	return value, err
}
func (p *Pipeline) AddUserToUser(value config.UserToUserConfig) error {
	return p.mutate("user-to-user", value.Name, "add", value)
}
func (p *Pipeline) UpdateUserToUser(name string, patch UserToUserPatch) error {
	return p.mutate("user-to-user", name, "update", patch)
}
func (p *Pipeline) DeleteUserToUser(name string) error {
	return p.mutate("user-to-user", name, "delete", nil)
}

// NonPersonalizedPatch distinguishes omitted fields from explicit zero values.
type NonPersonalizedPatch struct {
	Score  *string `json:"score,omitempty"`
	Filter *string `json:"filter,omitempty"`
}

func (p *Pipeline) ListNonPersonalized() ([]config.NonPersonalizedConfig, error) {
	values := make([]config.NonPersonalizedConfig, 0)
	err := p.list("non-personalized", &values)
	return values, err
}
func (p *Pipeline) GetNonPersonalized(name string) (config.NonPersonalizedConfig, error) {
	var value config.NonPersonalizedConfig
	err := p.get("non-personalized", name, &value)
	return value, err
}
func (p *Pipeline) AddNonPersonalized(value config.NonPersonalizedConfig) error {
	return p.mutate("non-personalized", value.Name, "add", value)
}
func (p *Pipeline) UpdateNonPersonalized(name string, patch NonPersonalizedPatch) error {
	return p.mutate("non-personalized", name, "update", patch)
}
func (p *Pipeline) DeleteNonPersonalized(name string) error {
	return p.mutate("non-personalized", name, "delete", nil)
}

// ExternalPatch distinguishes omitted fields from explicit zero values.
type ExternalPatch struct {
	Script *string `json:"script,omitempty"`
}

func (p *Pipeline) ListExternal() ([]config.ExternalConfig, error) {
	values := make([]config.ExternalConfig, 0)
	err := p.list("external", &values)
	return values, err
}
func (p *Pipeline) GetExternal(name string) (config.ExternalConfig, error) {
	var value config.ExternalConfig
	err := p.get("external", name, &value)
	return value, err
}
func (p *Pipeline) AddExternal(value config.ExternalConfig) error {
	return p.mutate("external", value.Name, "add", value)
}
func (p *Pipeline) UpdateExternal(name string, patch ExternalPatch) error {
	return p.mutate("external", name, "update", patch)
}
func (p *Pipeline) DeleteExternal(name string) error {
	return p.mutate("external", name, "delete", nil)
}

// CollaborativePatch updates the singleton collaborative configuration.
type CollaborativePatch struct {
	Type           *string        `json:"type,omitempty"`
	FitPeriod      *time.Duration `json:"fit_period,omitempty"`
	FitEpoch       *int           `json:"fit_epoch,omitempty"`
	OptimizePeriod *time.Duration `json:"optimize_period,omitempty"`
	OptimizeTrials *int           `json:"optimize_trials,omitempty"`
}

func (p *Pipeline) GetCollaborative() (config.CollaborativeConfig, error) {
	var value config.CollaborativeConfig
	_, _, rec, err := p.read()
	if err != nil {
		return value, err
	}
	current, ok := rec["collaborative"]
	if !ok {
		return value, fmt.Errorf("collaborative config not found")
	}
	err = decodePipeline(current, &value)
	return value, err
}
func (p *Pipeline) UpdateCollaborative(patch CollaborativePatch) error {
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err = json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) == 0 {
		return fmt.Errorf("no changes supplied")
	}
	// Preserve exact duration values instead of passing through JSON float64.
	if patch.FitPeriod != nil {
		fields["fit_period"] = patch.FitPeriod.String()
	}
	if patch.OptimizePeriod != nil {
		fields["optimize_period"] = patch.OptimizePeriod.String()
	}
	root, _, rec, err := p.read()
	if err != nil {
		return err
	}
	value, exists := rec["collaborative"]
	if !exists {
		return fmt.Errorf("collaborative config not found")
	}
	current, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("collaborative config must be an object")
	}
	for key, value := range fields {
		current[key] = value
	}
	_, err = p.client.UpdateConfig(root)
	return err
}

func init() {
	PipelineCmd.AddCommand(pipelineGetCmd)
	PipelineCmd.AddCommand(pipelineSchemaCmd)
	PipelineCmd.AddCommand(pipelinePatchCmd)
	PipelineCmd.AddCommand(pipelineResetCmd)
	PipelineCmd.AddCommand(newPipelineItemToItemCmd(), newPipelineUserToUserCmd(), newPipelineNonPersonalizedCmd(), newPipelineExternalCmd(), newPipelineCollaborativeCmd())

	for _, cmd := range []*cobra.Command{pipelineGetCmd, pipelineSchemaCmd, pipelinePatchCmd, pipelineResetCmd} {
		cmd.Flags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
		cmd.Flags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
		cmd.Flags().String("application", "", "Gorse CLI application name")
	}
}
