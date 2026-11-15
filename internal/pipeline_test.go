package internal

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type pipelineMemory struct {
	value  map[string]any
	writes int
	err    error
}

func (m *pipelineMemory) GetConfigMap() (map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	b, err := json.Marshal(m.value)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	err = json.Unmarshal(b, &v)
	return v, err
}
func (m *pipelineMemory) UpdateConfig(v map[string]any) (map[string]any, error) {
	m.writes++
	m.value = v
	return v, m.err
}
func (m *pipelineMemory) GetConfigSchema() (map[string]any, error) { return m.value, m.err }
func (m *pipelineMemory) ResetConfig() (map[string]any, error)     { return nil, m.err }
func pointer[T any](v T) *T                                        { return &v }
func pipelineFixture() *pipelineMemory {
	return &pipelineMemory{value: map[string]any{"future_root": true, "recommend": map[string]any{
		"future": "keep", "item-to-item": []any{map[string]any{"name": "first", "type": "users", "column": "item.Labels", "future": 42}, map[string]any{"name": "second", "type": "auto", "column": ""}},
		"user-to-user": []any{}, "non-personalized": []any{}, "external": []any{},
		"collaborative": map[string]any{"type": "mf", "fit_period": "1h", "fit_epoch": 10, "optimize_period": "1h", "optimize_trials": 10, "future": true},
		"ranker":        map[string]any{"recommenders": []any{}}, "fallback": map[string]any{"recommenders": []any{}},
	}}}
}

func TestPipelineItemLifecycle(t *testing.T) {
	m := pipelineFixture()
	p := NewPipeline(m)
	list, err := p.ListItemToItem()
	require.NoError(t, err)
	require.Equal(t, "first", list[0].Name)
	got, err := p.GetItemToItem("first")
	require.NoError(t, err)
	require.Equal(t, "users", got.Type)
	require.NoError(t, p.UpdateItemToItem("first", ItemToItemPatch{Column: pointer("")}))
	got, err = p.GetItemToItem("first")
	require.NoError(t, err)
	require.Empty(t, got.Column)
	require.Equal(t, "users", got.Type)
	r := m.value["recommend"].(map[string]any)
	require.Equal(t, float64(42), r["item-to-item"].([]any)[0].(map[string]any)["future"])
	require.Equal(t, "keep", r["future"])
	require.Equal(t, true, m.value["future_root"])
	require.NoError(t, p.AddItemToItem(ItemToItemConfig{Name: "third", Type: "auto"}))
	list, err = p.ListItemToItem()
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second", "third"}, []string{list[0].Name, list[1].Name, list[2].Name})
	require.NoError(t, p.DeleteItemToItem("second"))
	list, err = p.ListItemToItem()
	require.NoError(t, err)
	require.Equal(t, "third", list[1].Name)
	_, err = p.GetItemToItem("second")
	require.ErrorContains(t, err, "not found")
}
func TestPipelineStrictMutations(t *testing.T) {
	m := pipelineFixture()
	p := NewPipeline(m)
	require.ErrorContains(t, p.AddItemToItem(ItemToItemConfig{Name: "first", Type: "auto"}), "already exists")
	require.ErrorContains(t, p.AddItemToItem(ItemToItemConfig{Name: " ", Type: "auto"}), "name")
	require.ErrorContains(t, p.UpdateItemToItem("first", ItemToItemPatch{}), "no changes")
	require.ErrorContains(t, p.UpdateItemToItem("missing", ItemToItemPatch{Type: pointer("auto")}), "not found")
	require.ErrorContains(t, p.DeleteItemToItem("missing"), "not found")
	require.Zero(t, m.writes)
	// Even an ambiguous existing configuration must not be silently modified.
	r := m.value["recommend"].(map[string]any)
	r["item-to-item"] = append(r["item-to-item"].([]any), map[string]any{"name": "first", "type": "auto"})
	require.ErrorContains(t, p.DeleteItemToItem("first"), "duplicate")
	require.Zero(t, m.writes)
}
func TestPipelineDeleteReferences(t *testing.T) {
	for _, section := range []string{"ranker", "fallback"} {
		t.Run(section, func(t *testing.T) {
			m := pipelineFixture()
			r := m.value["recommend"].(map[string]any)
			r[section] = map[string]any{"recommenders": []any{"item-to-item/first"}}
			require.ErrorContains(t, NewPipeline(m).DeleteItemToItem("first"), "recommend."+section+".recommenders[0]")
			require.Zero(t, m.writes)
		})
	}
}
func TestPipelineOtherResources(t *testing.T) {
	m := pipelineFixture()
	p := NewPipeline(m)
	require.NoError(t, p.AddUserToUser(UserToUserConfig{Name: "u", Type: "items", Column: "item.Labels"}))
	require.NoError(t, p.UpdateUserToUser("u", UserToUserPatch{Column: pointer("")}))
	u, err := p.GetUserToUser("u")
	require.NoError(t, err)
	require.Empty(t, u.Column)
	us, err := p.ListUserToUser()
	require.NoError(t, err)
	require.Len(t, us, 1)
	require.NoError(t, p.DeleteUserToUser("u"))
	require.NoError(t, p.AddNonPersonalized(NonPersonalizedConfig{Name: "n", Score: "1", Filter: "true"}))
	require.NoError(t, p.UpdateNonPersonalized("n", NonPersonalizedPatch{Filter: pointer("")}))
	n, err := p.GetNonPersonalized("n")
	require.NoError(t, err)
	require.Empty(t, n.Filter)
	ns, err := p.ListNonPersonalized()
	require.NoError(t, err)
	require.Len(t, ns, 1)
	require.NoError(t, p.DeleteNonPersonalized("n"))
	require.NoError(t, p.AddExternal(ExternalConfig{Name: "e", Script: "old"}))
	require.NoError(t, p.UpdateExternal("e", ExternalPatch{Script: pointer("")}))
	e, err := p.GetExternal("e")
	require.NoError(t, err)
	require.Empty(t, e.Script)
	es, err := p.ListExternal()
	require.NoError(t, err)
	require.Len(t, es, 1)
	require.NoError(t, p.DeleteExternal("e"))
	require.NoError(t, p.UpdateCollaborative(CollaborativePatch{Type: pointer("none"), OptimizePeriod: pointer(time.Duration(0))}))
	c, err := p.GetCollaborative()
	require.NoError(t, err)
	require.Equal(t, "none", c.Type)
	require.Zero(t, c.OptimizePeriod)
	require.Equal(t, time.Hour, c.FitPeriod)
	require.Equal(t, true, m.value["recommend"].(map[string]any)["collaborative"].(map[string]any)["future"])
	require.ErrorContains(t, p.UpdateCollaborative(CollaborativePatch{}), "no changes")
}
func TestPipelineMissingCollaborative(t *testing.T) {
	m := pipelineFixture()
	delete(m.value["recommend"].(map[string]any), "collaborative")
	p := NewPipeline(m)
	_, err := p.GetCollaborative()
	require.ErrorContains(t, err, "collaborative config not found")
	require.ErrorContains(t, p.UpdateCollaborative(CollaborativePatch{Type: pointer("none")}), "collaborative config not found")
	require.Zero(t, m.writes)
}

func TestPipelineTransportErrors(t *testing.T) {
	m := pipelineFixture()
	m.err = errors.New("offline")
	p := NewPipeline(m)
	_, err := p.ListExternal()
	require.ErrorContains(t, err, "offline")
	require.ErrorContains(t, p.AddExternal(ExternalConfig{Name: "e"}), "offline")
	_, err = p.Get()
	require.ErrorContains(t, err, "offline")
}

func TestPipelineTypedConfigRoundTrip(t *testing.T) {
	m := pipelineFixture()
	recommend := m.value["recommend"].(map[string]any)
	recommend["data_source"] = map[string]any{
		"positive_feedback_types": []any{"star", "read>=3"},
		"negative_feedback_types": []any{"dislike"},
		"read_feedback_types":     []any{"read"},
	}
	recommend["cache_expire"] = "72h"
	recommend["collaborative"].(map[string]any)["early_stopping"] = map[string]any{"patience": 5}
	before, err := m.GetConfigMap()
	require.NoError(t, err)

	p := NewPipeline(m)
	period := time.Duration(1<<53 + 1)
	require.NoError(t, p.UpdateCollaborative(CollaborativePatch{FitPeriod: &period}))
	got, err := p.GetCollaborative()
	require.NoError(t, err)
	require.Equal(t, period, got.FitPeriod)
	require.Equal(t, 5, got.EarlyStopping.Patience)
	recommend = m.value["recommend"].(map[string]any)
	require.Equal(t, period.String(), recommend["collaborative"].(map[string]any)["fit_period"])
	require.Equal(t, before["recommend"].(map[string]any)["data_source"], recommend["data_source"])
	require.Equal(t, "72h", recommend["cache_expire"])
	require.Equal(t, true, recommend["collaborative"].(map[string]any)["future"])
}

func TestPipelineDeletePreservesRemainingFields(t *testing.T) {
	m := pipelineFixture()
	p := NewPipeline(m)
	require.NoError(t, p.DeleteItemToItem("second"))
	recommend := m.value["recommend"].(map[string]any)
	entries := recommend["item-to-item"].([]any)
	require.Len(t, entries, 1)
	require.Equal(t, "first", entries[0].(map[string]any)["name"])
	require.Equal(t, float64(42), entries[0].(map[string]any)["future"])
	require.NoError(t, p.DeleteItemToItem("first"))
	list, err := p.ListItemToItem()
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestPipelineRejectsMalformedTypedConfig(t *testing.T) {
	for _, test := range []struct {
		name  string
		field string
		value any
	}{
		{name: "invalid list", field: "item-to-item", value: "invalid"},
		{name: "invalid entry", field: "item-to-item", value: []any{"invalid"}},
		{name: "missing name", field: "item-to-item", value: []any{map[string]any{"type": "users"}}},
		{name: "invalid duration", field: "collaborative", value: map[string]any{"fit_period": "invalid"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := pipelineFixture()
			m.value["recommend"].(map[string]any)[test.field] = test.value
			require.Error(t, NewPipeline(m).AddExternal(ExternalConfig{Name: "new"}))
			require.Zero(t, m.writes)
		})
	}
}
