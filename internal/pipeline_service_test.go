package internal

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gorse-io/gorse/config"
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
	require.NoError(t, p.AddItemToItem(config.ItemToItemConfig{Name: "third", Type: "auto"}))
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
	require.ErrorContains(t, p.AddItemToItem(config.ItemToItemConfig{Name: "first", Type: "auto"}), "already exists")
	require.ErrorContains(t, p.AddItemToItem(config.ItemToItemConfig{Name: " ", Type: "auto"}), "name")
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
	require.NoError(t, p.AddUserToUser(config.UserToUserConfig{Name: "u", Type: "items", Column: "item.Labels"}))
	require.NoError(t, p.UpdateUserToUser("u", UserToUserPatch{Column: pointer("")}))
	u, err := p.GetUserToUser("u")
	require.NoError(t, err)
	require.Empty(t, u.Column)
	us, err := p.ListUserToUser()
	require.NoError(t, err)
	require.Len(t, us, 1)
	require.NoError(t, p.DeleteUserToUser("u"))
	require.NoError(t, p.AddNonPersonalized(config.NonPersonalizedConfig{Name: "n", Score: "1", Filter: "true"}))
	require.NoError(t, p.UpdateNonPersonalized("n", NonPersonalizedPatch{Filter: pointer("")}))
	n, err := p.GetNonPersonalized("n")
	require.NoError(t, err)
	require.Empty(t, n.Filter)
	ns, err := p.ListNonPersonalized()
	require.NoError(t, err)
	require.Len(t, ns, 1)
	require.NoError(t, p.DeleteNonPersonalized("n"))
	require.NoError(t, p.AddExternal(config.ExternalConfig{Name: "e", Script: "old"}))
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
	require.ErrorContains(t, p.AddExternal(config.ExternalConfig{Name: "e"}), "offline")
	_, err = p.Get()
	require.ErrorContains(t, err, "offline")
}
