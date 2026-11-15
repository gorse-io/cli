package internal

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
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
	if m.err != nil {
		return nil, m.err
	}
	m.writes++
	// The server replaces only the recommendation section of the configuration.
	m.value["recommend"] = v["recommend"]
	return m.value, m.err
}
func pipelineFixture() *pipelineMemory {
	return &pipelineMemory{value: map[string]any{"future_root": true, "recommend": map[string]any{
		"future": "keep", "item-to-item": []any{map[string]any{"name": "first", "type": "users", "column": "item.Labels", "future": 42}, map[string]any{"name": "second", "type": "auto", "column": ""}},
		"user-to-user": []any{}, "non-personalized": []any{}, "external": []any{},
		"collaborative": map[string]any{"type": "mf", "fit_period": "1h", "fit_epoch": 10, "optimize_period": "1h", "optimize_trials": 10, "future": true},
		"ranker":        map[string]any{"recommenders": []any{}}, "fallback": map[string]any{"recommenders": []any{}},
	}}}
}

func pipelineCommandRoot(t *testing.T, memory *pipelineMemory) *cobra.Command {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dashboard/config" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}
		var value map[string]any
		var err error
		switch r.Method {
		case http.MethodGet:
			value, err = memory.GetConfigMap()
		case http.MethodPost:
			var update map[string]any
			if err = json.NewDecoder(r.Body).Decode(&update); err == nil {
				value, err = memory.UpdateConfig(update)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}))
	t.Cleanup(server.Close)
	t.Setenv("GORSE_ADMIN_ENDPOINT", server.URL)
	t.Setenv("GORSE_ADMIN_API_KEY", "test-key")
	root := &cobra.Command{Use: "gorse-cli", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
	root.PersistentFlags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
	root.PersistentFlags().String("application", "", "Gorse CLI application name")
	root.AddCommand(PipelineCmd)
	t.Cleanup(func() {
		resetPipelineFlags(root)
		root.RemoveCommand(PipelineCmd)
	})
	return root
}

func resetPipelineFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	})
	for _, child := range cmd.Commands() {
		resetPipelineFlags(child)
	}
}

func runPipelineCommand(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	resetPipelineFlags(root)
	stdout, stderr := Command(t, root, append([]string{"pipeline"}, args...)...)
	require.Empty(t, stderr)
	return stdout
}

func pipelineCommandError(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	resetPipelineFlags(root)
	root.SetArgs(append([]string{"pipeline"}, args...))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	require.Error(t, err)
	return err
}

func TestPipelineInheritedCredentialFlags(t *testing.T) {
	for _, test := range []struct {
		name        string
		before      bool
		application bool
	}{
		{name: "flags before commands", before: true},
		{name: "flags after commands"},
		{name: "selected application", before: true, application: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			memory := pipelineFixture()
			root := pipelineCommandRoot(t, memory)
			endpoint := os.Getenv("GORSE_ADMIN_ENDPOINT")
			flags := []string{"--endpoint", endpoint, "--api-key", "test-key"}
			if test.application {
				// The inherited application flag selects saved credentials without environment defaults.
				keyring.MockInit()
				require.NoError(t, applications.Upsert("dev", endpoint, "test-key"))
				t.Setenv("GORSE_ADMIN_ENDPOINT", "")
				t.Setenv("GORSE_ADMIN_API_KEY", "")
				flags = []string{"--application", "dev"}
			} else {
				// Explicit inherited flags override conflicting environment credentials.
				t.Setenv("GORSE_ADMIN_ENDPOINT", "http://127.0.0.1:1")
				t.Setenv("GORSE_ADMIN_API_KEY", "wrong-key")
			}
			args := []string{"pipeline", "external", "list"}
			if test.before {
				args = append(flags, args...)
			} else {
				args = append(args, flags...)
			}
			resetPipelineFlags(root)
			stdout, stderr := Command(t, root, args...)
			require.Equal(t, "[]\n", stdout)
			require.Empty(t, stderr)
			require.Zero(t, memory.writes)
		})
	}
}

func TestPipelineRecommenderCommands(t *testing.T) {
	for _, test := range []struct {
		kind     string
		add      []string
		update   []string
		cleared  string
		retained string
		value    string
	}{
		{kind: "item-to-item", add: []string{"--type", "users", "--column", "item.Labels"}, update: []string{"--column="}, cleared: "column", retained: "type", value: "users"},
		{kind: "user-to-user", add: []string{"--type", "items", "--column", "item.Labels"}, update: []string{"--column="}, cleared: "column", retained: "type", value: "items"},
		{kind: "non-personalized", add: []string{"--score", "1", "--filter", "true"}, update: []string{"--filter="}, cleared: "filter", retained: "score", value: "1"},
		{kind: "external", add: []string{"--script", "return [];"}, update: []string{"--script="}, cleared: "script", retained: "name", value: "managed"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			memory := pipelineFixture()
			root := pipelineCommandRoot(t, memory)
			// Adding a recommender is persisted and visible through both get and list.
			out := runPipelineCommand(t, root, append([]string{test.kind, "add", "managed"}, test.add...)...)
			require.Equal(t, "add succeeded: managed\n", out)
			var value map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(runPipelineCommand(t, root, test.kind, "get", "managed")), &value))
			require.Equal(t, "managed", value["name"])
			require.Equal(t, test.value, value[test.retained])
			require.Contains(t, runPipelineCommand(t, root, test.kind, "list"), "managed")

			// An explicit empty flag clears its field while omitted fields remain unchanged.
			runPipelineCommand(t, root, append([]string{test.kind, "update", "managed"}, test.update...)...)
			require.NoError(t, yaml.Unmarshal([]byte(runPipelineCommand(t, root, test.kind, "get", "managed")), &value))
			require.Empty(t, value[test.cleared])
			require.Equal(t, test.value, value[test.retained])

			// Deletion removes only the selected recommender and leaves reads without writes.
			runPipelineCommand(t, root, test.kind, "delete", "managed")
			require.NotContains(t, runPipelineCommand(t, root, test.kind, "list"), "managed")
			require.Equal(t, 3, memory.writes)
			if test.kind == "item-to-item" {
				p := NewPipeline(memory)
				require.NoError(t, p.Load())
				require.Len(t, p.ItemToItem, 2)
				require.Equal(t, ItemToItemConfig{Name: "first", Type: "users", Column: "item.Labels"}, p.ItemToItem[0])
			}
		})
	}
}

func TestPipelineInvalidRecommenderCommands(t *testing.T) {
	for _, test := range []struct {
		kind   string
		flags  []string
		change []string
	}{
		{kind: "item-to-item", flags: []string{"--type", "users"}, change: []string{"--column="}},
		{kind: "user-to-user", flags: []string{"--type", "items"}, change: []string{"--column="}},
		{kind: "non-personalized", flags: []string{"--score", "1"}, change: []string{"--filter="}},
		{kind: "external", flags: []string{"--script", "return [];"}, change: []string{"--script="}},
	} {
		t.Run(test.kind, func(t *testing.T) {
			memory := pipelineFixture()
			memory.value["recommend"].(map[string]any)[test.kind] = []any{map[string]any{"name": "existing"}}
			root := pipelineCommandRoot(t, memory)
			// Duplicate names, missing targets, and empty updates must not write configuration.
			err := pipelineCommandError(t, root, append([]string{test.kind, "add", "existing"}, test.flags...)...)
			require.ErrorContains(t, err, "already exists")
			for _, operation := range []string{"get", "update", "delete"} {
				args := []string{test.kind, operation, "missing"}
				if operation == "update" {
					args = append(args, test.change...)
				}
				require.ErrorContains(t, pipelineCommandError(t, root, args...), "not found")
			}
			require.ErrorContains(t, pipelineCommandError(t, root, test.kind, "update", "existing"), "no changes")
			require.ErrorContains(t, pipelineCommandError(t, root, test.kind, "add", "new"), "required")

			// Whitespace names are rejected for every named operation.
			for _, operation := range []string{"get", "add", "update", "delete"} {
				args := []string{test.kind, operation, " "}
				if operation == "add" {
					args = append(args, test.flags...)
				} else if operation == "update" {
					args = append(args, test.change...)
				}
				require.ErrorContains(t, pipelineCommandError(t, root, args...), "name must not be empty")
			}
			require.Zero(t, memory.writes)
		})
	}
}

func TestPipelineDeleteReferences(t *testing.T) {
	for _, kind := range []string{"item-to-item", "user-to-user", "non-personalized", "external"} {
		for _, section := range []string{"ranker", "fallback"} {
			t.Run(kind+"/"+section, func(t *testing.T) {
				memory := pipelineFixture()
				recommend := memory.value["recommend"].(map[string]any)
				recommend[kind] = []any{map[string]any{"name": "protected"}}
				recommend[section] = map[string]any{"recommenders": []any{kind + "/protected"}}
				root := pipelineCommandRoot(t, memory)
				// Referenced recommenders cannot be deleted from any resource collection.
				err := pipelineCommandError(t, root, kind, "delete", "protected")
				require.ErrorContains(t, err, "recommend."+section+".recommenders[0]")
				require.Zero(t, memory.writes)
			})
		}
	}
}

func TestPipelineExternalScriptFile(t *testing.T) {
	memory := pipelineFixture()
	root := pipelineCommandRoot(t, memory)
	path := filepath.Join(t.TempDir(), "script.js")
	require.NoError(t, os.WriteFile(path, []byte("return [];\n"), 0600))
	// A script file is read verbatim, and conflicting script flags are rejected.
	runPipelineCommand(t, root, "external", "add", "managed", "--script-file", path)
	p := NewPipeline(memory)
	require.NoError(t, p.Load())
	require.Equal(t, []ExternalConfig{{Name: "managed", Script: "return [];\n"}}, p.External)
	err := pipelineCommandError(t, root, "external", "update", "managed", "--script-file", path, "--script", "return [];")
	require.Error(t, err)
	require.Equal(t, 1, memory.writes)
}

func TestPipelineCollaborativeCommand(t *testing.T) {
	memory := pipelineFixture()
	root := pipelineCommandRoot(t, memory)
	// Explicit zero values are persisted while unspecified fields keep their values.
	runPipelineCommand(t, root, "collaborative", "update", "--type", "none", "--fit-period", "2h", "--optimize-period", "0s")
	p := NewPipeline(memory)
	require.NoError(t, p.Load())
	require.Equal(t, "none", p.Collaborative.Type)
	require.Equal(t, 2*time.Hour, p.Collaborative.FitPeriod)
	require.Zero(t, p.Collaborative.OptimizePeriod)
	require.Equal(t, 10, p.Collaborative.FitEpoch)
	// A later update changes only the flags supplied to that invocation.
	runPipelineCommand(t, root, "collaborative", "update", "--fit-epoch", "20")
	require.NoError(t, p.Load())
	require.Equal(t, 20, p.Collaborative.FitEpoch)
	require.Equal(t, "none", p.Collaborative.Type)
	require.Equal(t, 2*time.Hour, p.Collaborative.FitPeriod)
	require.Contains(t, runPipelineCommand(t, root, "collaborative", "get"), "none")
	require.ErrorContains(t, pipelineCommandError(t, root, "collaborative", "update"), "no changes")
	require.Equal(t, 2, memory.writes)
}

func TestPipelineGetCommand(t *testing.T) {
	memory := pipelineFixture()
	root := pipelineCommandRoot(t, memory)
	// Whole-pipeline output retains the API field names and formatted durations.
	var value map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(runPipelineCommand(t, root, "get")), &value))
	require.Contains(t, value, "item-to-item")
	require.Equal(t, "1h", value["collaborative"].(map[string]any)["fit_period"])
	require.Zero(t, memory.writes)
}

func TestPipelineMissingCollaborative(t *testing.T) {
	memory := pipelineFixture()
	delete(memory.value["recommend"].(map[string]any), "collaborative")
	p := NewPipeline(memory)
	require.ErrorContains(t, p.Load(), "collaborative config not found")
	require.Zero(t, memory.writes)
}

func TestPipelineTransportErrors(t *testing.T) {
	memory := pipelineFixture()
	memory.err = errors.New("offline")
	p := NewPipeline(memory)
	// Both loading and saving propagate transport errors without successful writes.
	require.ErrorContains(t, p.Load(), "offline")
	require.ErrorContains(t, p.Save(), "offline")
	require.Zero(t, memory.writes)
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
	p := NewPipeline(m)
	period := time.Duration(1<<53 + 1)
	// Saving a direct field change preserves nanosecond precision and other known fields.
	require.NoError(t, p.Load())
	dataSource := p.DataSource
	p.Collaborative.FitPeriod = period
	require.NoError(t, p.Save())
	require.NoError(t, p.Load())
	require.Equal(t, period, p.Collaborative.FitPeriod)
	require.Equal(t, 5, p.Collaborative.EarlyStopping.Patience)
	recommend = m.value["recommend"].(map[string]any)
	require.Equal(t, period.String(), recommend["collaborative"].(map[string]any)["fit_period"])
	require.Equal(t, dataSource, p.DataSource)
	cacheExpire, err := time.ParseDuration(recommend["cache_expire"].(string))
	require.NoError(t, err)
	require.Equal(t, 72*time.Hour, cacheExpire)
}

func TestPipelineLoadReplacesLocalFields(t *testing.T) {
	m := pipelineFixture()
	recommend := m.value["recommend"].(map[string]any)
	recommend["data_source"] = map[string]any{"positive_feedback_types": []any{"star"}}
	p := NewPipeline(m)
	require.NoError(t, p.Load())
	require.Len(t, p.ItemToItem, 2)
	require.Equal(t, []string{"star"}, p.DataSource.PositiveFeedbackTypes)

	// Reloading reflects remote removals instead of retaining previous local values.
	recommend["item-to-item"] = []any{}
	delete(recommend, "data_source")
	require.NoError(t, p.Load())
	require.Empty(t, p.ItemToItem)
	require.Empty(t, p.DataSource.PositiveFeedbackTypes)
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
		{name: "duplicate names", field: "item-to-item", value: []any{map[string]any{"name": "same"}, map[string]any{"name": "same"}}},
		{name: "invalid duration", field: "collaborative", value: map[string]any{"fit_period": "invalid"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := pipelineFixture()
			m.value["recommend"].(map[string]any)[test.field] = test.value
			require.Error(t, NewPipeline(m).Load())
			require.Zero(t, m.writes)
		})
	}
}
