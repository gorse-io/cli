package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/gorse-io/gorse/client"
	"github.com/stretchr/testify/require"
)

func (s *CLITestSuite) TestGet() {
	var fields map[string]any
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &fields, Deep: true})
	s.Require().NoError(err)
	s.Require().NoError(decoder.Decode(s.master.Config.Recommend))
	expected, err := json.Marshal(formatConfigMap(fields))
	s.Require().NoError(err)
	out, err := s.Command("pipeline", "get")
	s.Require().NoError(err)
	s.Require().YAMLEq(string(expected), out)
}

// Each settings test restores configuration: the shared fixture only purges data.
func (s *CLITestSuite) pipelineSettingsFixture() {
	s.T().Helper()
	s.master.ConfigMutex.Lock()
	original := s.master.Config.Recommend
	// Configuration writes schedule training. Disable it for these CLI-only tests.
	s.master.Config.Recommend.Collaborative.Type = "none"
	s.master.Config.Recommend.Ranker.Type = "none"
	s.master.Config.Recommend.Ranker.RerankerAPI.AuthToken = "reranker-secret"
	s.master.Config.Recommend.Ranker.DecisionAPI.AuthToken = "decision-secret"
	s.master.Config.Recommend.Ranker.RerankerAPI.Model = "keep-model"
	s.master.Config.Recommend.Ranker.DecisionAPI.StateTemplate = "keep-state"
	s.master.ConfigMutex.Unlock()
	s.T().Cleanup(func() {
		s.master.ConfigMutex.Lock()
		s.master.Config.Recommend = original
		s.master.ConfigMutex.Unlock()
	})
}

func (s *CLITestSuite) pipelineSettings() map[string]any {
	s.T().Helper()
	root, err := client.NewAdminClient(fmt.Sprintf("http://127.0.0.1:%d", s.master.Config.Master.HttpPort), "test-key").GetConfigMap()
	s.Require().NoError(err)
	return root["recommend"].(map[string]any)
}

func (s *CLITestSuite) TestPipelineSettings() {
	s.pipelineSettingsFixture()
	tests := []struct {
		section string
		args    []string
		path    []string
		value   any
	}{
		{"", []string{"--cache-size=123"}, []string{"cache_size"}, float64(123)},
		{"", []string{"--cache-expire=1h2ns"}, []string{"cache_expire"}, "1h0m0.000000002s"},
		{"", []string{"--context-size=17"}, []string{"context_size"}, float64(17)},
		{"", []string{"--active-user-ttl=7"}, []string{"active_user_ttl"}, float64(7)},
		{"", []string{"--active-user-ttl=0"}, []string{"active_user_ttl"}, float64(0)},
		{"data-source", []string{"--positive-feedback-type=like", "--positive-feedback-type=click"}, []string{"data_source", "positive_feedback_types"}, []any{"like", "click"}},
		{"data-source", []string{"--negative-feedback-type=dislike", "--negative-feedback-type=hide"}, []string{"data_source", "negative_feedback_types"}, []any{"dislike", "hide"}},
		{"data-source", []string{"--read-feedback-type=read", "--read-feedback-type=view"}, []string{"data_source", "read_feedback_types"}, []any{"read", "view"}},
		{"data-source", []string{"--positive-feedback-ttl=3"}, []string{"data_source", "positive_feedback_ttl"}, float64(3)},
		{"data-source", []string{"--positive-feedback-ttl=0"}, []string{"data_source", "positive_feedback_ttl"}, float64(0)},
		{"data-source", []string{"--item-ttl=2"}, []string{"data_source", "item_ttl"}, float64(2)},
		{"data-source", []string{"--item-ttl=0"}, []string{"data_source", "item_ttl"}, float64(0)},
		{"search", []string{"--column=item.Labels.title", "--column=item.Labels.body"}, []string{"search", "columns"}, []any{"item.Labels.title", "item.Labels.body"}},
		{"ranker", []string{"--type=none"}, []string{"ranker", "type"}, "none"},
		{"ranker", []string{"--type=fm"}, []string{"ranker", "type"}, "fm"},
		{"ranker", []string{"--recommender=latest", "--recommender=item-to-item/first"}, []string{"ranker", "recommenders"}, []any{"latest", "item-to-item/first"}},
		{"ranker", []string{"--cache-expire=2h3ns"}, []string{"ranker", "cache_expire"}, "2h0m0.000000003s"},
		{"ranker", []string{"--fit-period=3h4ns"}, []string{"ranker", "fit_period"}, "3h0m0.000000004s"},
		{"ranker", []string{"--fit-epoch=2"}, []string{"ranker", "fit_epoch"}, float64(2)},
		{"ranker", []string{"--optimize-period=4h5ns"}, []string{"ranker", "optimize_period"}, "4h0m0.000000005s"},
		{"ranker", []string{"--optimize-period=0s"}, []string{"ranker", "optimize_period"}, "0s"},
		{"ranker", []string{"--optimize-trials=2"}, []string{"ranker", "optimize_trials"}, float64(2)},
		{"ranker", []string{"--early-stopping-patience=3"}, []string{"ranker", "early_stopping", "patience"}, float64(3)},
		{"ranker", []string{"--early-stopping-patience=0"}, []string{"ranker", "early_stopping", "patience"}, float64(0)},
		{"fallback", []string{"--recommender=item-to-item/first", "--recommender=latest"}, []string{"fallback", "recommenders"}, []any{"item-to-item/first", "latest"}},
		{"replacement", []string{"--enable-replacement=true"}, []string{"replacement", "enable_replacement"}, true},
		{"replacement", []string{"--positive-replacement-decay=0.8"}, []string{"replacement", "positive_replacement_decay"}, 0.8},
		{"replacement", []string{"--read-replacement-decay=0.7"}, []string{"replacement", "read_replacement_decay"}, 0.7},
		{"replacement", []string{"--enable-replacement=false"}, []string{"replacement", "enable_replacement"}, false},
	}
	for _, provider := range []struct {
		name   string
		fields []string
	}{
		{"reranker", []string{"url", "model", "auth-token", "query-template", "document-template"}},
		{"decision", []string{"url", "model", "auth-token", "state-template", "instructions-template"}},
	} {
		for _, field := range provider.fields {
			key := strings.ReplaceAll(field, "-", "_")
			tests = append(tests, struct {
				section string
				args    []string
				path    []string
				value   any
			}{"ranker", []string{"--" + provider.name + "-" + field + "=updated"}, []string{"ranker", provider.name + "_api", key}, "updated"})
		}
	}
	// Empty string is a change, not omission.
	tests = append(tests, struct {
		section string
		args    []string
		path    []string
		value   any
	}{"ranker", []string{"--reranker-model="}, []string{"ranker", "reranker_api", "model"}, ""})
	for _, tc := range tests {
		s.Run(tc.section+fmt.Sprint(tc.args), func() {
			expected := s.pipelineSettings()
			target := expected
			for _, key := range tc.path[:len(tc.path)-1] {
				target = target[key].(map[string]any)
			}
			target[tc.path[len(tc.path)-1]] = tc.value
			args := []string{"pipeline"}
			if tc.section != "" {
				args = append(args, tc.section)
			}
			args = append(args, "update")
			out, err := s.Command(append(args, tc.args...)...)
			s.Require().NoError(err)
			s.NotContains(out, "secret")
			s.Equal("Pipeline settings updated.\n", out) // Do not echo provider credentials.
			s.Equal(expected, s.pipelineSettings(), "only requested field should change")
		})
	}
	for _, section := range []string{"data-source", "search", "ranker", "fallback", "replacement"} {
		out, err := s.Command("pipeline", section, "get")
		s.Require().NoError(err)
		key := section
		if key == "data-source" {
			key = "data_source"
		}
		expected, err := json.Marshal(s.pipelineSettings()[key])
		s.Require().NoError(err)
		s.YAMLEq(string(expected), out)
	}
}

func (s *CLITestSuite) TestPipelineSettingsClearLists() {
	s.pipelineSettingsFixture()
	for _, tc := range []struct{ section, flag, key string }{
		{"data-source", "positive-feedback-type", "positive_feedback_types"},
		{"data-source", "negative-feedback-type", "negative_feedback_types"},
		{"data-source", "read-feedback-type", "read_feedback_types"},
		{"search", "column", "columns"},
		{"ranker", "recommender", "recommenders"},
		{"fallback", "recommender", "recommenders"},
	} {
		value := "latest"
		if tc.section == "data-source" {
			value = "click"
		}
		if tc.section == "search" {
			value = "item.Labels.title"
		}
		_, err := s.Command("pipeline", tc.section, "update", "--"+tc.flag+"="+value)
		s.Require().NoError(err)
		before := s.pipelineSettings()
		_, err = s.Command("pipeline", tc.section, "update", "--"+tc.flag+"=")
		if tc.section == "ranker" {
			// Master forbids an empty ranker list; it must receive [] rather than [""].
			s.Require().ErrorContains(err, "ranker.recommenders must not be empty")
			s.Equal(before, s.pipelineSettings())
			continue
		}
		s.Require().NoError(err)
		section := tc.section
		if section == "data-source" {
			section = "data_source"
		}
		s.Equal([]any{}, s.pipelineSettings()[section].(map[string]any)[tc.key])
	}
}

func (s *CLITestSuite) TestPipelineSettingsNoChanges() {
	for _, section := range []string{"", "data-source", "search", "ranker", "fallback", "replacement"} {
		args := []string{"pipeline"}
		if section != "" {
			args = append(args, section)
		}
		args = append(args, "update", "--endpoint=http://127.0.0.1:1", "--api-key=unused")
		_, err := s.Command(args...)
		s.Require().EqualError(err, "no changes supplied")
	}
}

func (s *CLITestSuite) TestPipelineSettingsRejected() {
	s.pipelineSettingsFixture()
	for _, args := range [][]string{
		{"update", "--cache-size=0"},
		{"update", "--cache-expire=0s"},
		{"data-source", "update", "--positive-feedback-type=("},
		{"search", "update", "--column=("},
		{"ranker", "update", "--type=invalid"},
		{"ranker", "update", "--optimize-trials=0"},
		{"replacement", "update", "--positive-replacement-decay=0"},
	} {
		before := s.pipelineSettings()
		_, err := s.Command(append([]string{"pipeline"}, args...)...)
		s.Require().Error(err)
		s.Contains(err.Error(), "400", "Master, not the CLI, validates settings")
		s.Equal(before, s.pipelineSettings())
	}
}

func (s *CLITestSuite) TestPipelineSettingsRedactedSecrets() {
	s.pipelineSettingsFixture()
	s.master.ConfigMutex.Lock()
	original := s.master.Config.Master.DashboardRedacted
	s.master.Config.Master.DashboardRedacted = true
	s.master.ConfigMutex.Unlock()
	defer func() {
		s.master.ConfigMutex.Lock()
		s.master.Config.Master.DashboardRedacted = original
		s.master.ConfigMutex.Unlock()
	}()
	_, err := s.Command("pipeline", "update", "--cache-size=211")
	s.Require().NoError(err)
	s.master.ConfigMutex.RLock()
	actual := s.master.Config.Recommend
	s.master.ConfigMutex.RUnlock()
	s.Equal("reranker-secret", actual.Ranker.RerankerAPI.AuthToken)
	s.Equal("decision-secret", actual.Ranker.DecisionAPI.AuthToken)
	s.Equal(211, actual.CacheSize)
}

// Only this wire-contract test uses a capture server: Master itself discards
// unknown config keys, so its persisted config cannot prove forward compatibility.
func TestPipelineSettingsPreserveUnknown(t *testing.T) {
	raw := map[string]any{"collaborative": map[string]any{"type": "none"}, "future": map[string]any{"nested": "keep"}, "ranker": map[string]any{"future_provider": "keep", "reranker_api": map[string]any{"auth_token": "secret", "future": "keep"}}}
	var posted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"recommend": raw}))
			return
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	c := client.NewAdminClient(server.URL, "key")
	var p Pipeline
	require.NoError(t, p.Load(c))
	// Exercise the same service used by the command, including nested merge.
	require.NoError(t, p.SaveChanges(c, map[string]any{"ranker": map[string]any{"reranker_api": map[string]any{"model": "new"}}, "cache_expire": (time.Hour + time.Nanosecond).String()}))
	expected := raw["ranker"].(map[string]any)["reranker_api"].(map[string]any)
	expected["model"] = "new"
	raw["cache_expire"] = "1h0m0.000000001s"
	require.Equal(t, map[string]any{"recommend": raw}, posted)
}
