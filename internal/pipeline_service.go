// Copyright 2026 gorse Project Authors
// Licensed under the Apache License, Version 2.0.

package internal

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/evanphx/json-patch/v5"
	"github.com/go-viper/mapstructure/v2"
	"github.com/gorse-io/gorse/config"
)

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
