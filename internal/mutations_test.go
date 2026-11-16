package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorse-io/gorse/storage"
	"github.com/stretchr/testify/require"
)

func (s *CLITestSuite) TestAddItem() {
	out, err := s.execute("item", "add", "added-item", "--comment", "new item",
		"--category", "news", "--category", "tech", "--is-hidden",
		"--timestamp", "2026-01-02T03:04:05Z", "--labels", `{"genre":"science","embedding":[0.1,0.2]}`)
	s.Require().NoError(err)
	s.Require().Contains(out, "RowAffected: 1")
	item, err := s.m.DataClient.GetItem(s.T().Context(), "added-item")
	s.Require().NoError(err)
	s.Require().Equal("new item", item.Comment)
	s.Require().Equal([]string{"news", "tech"}, item.Categories)
	s.Require().True(item.IsHidden)
	s.Require().Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), item.Timestamp)
	s.Require().Equal("science", item.Labels.(map[string]any)["genre"])

	_, err = s.execute("item", "add", "minimal-item")
	s.Require().NoError(err)
	item, err = s.m.DataClient.GetItem(s.T().Context(), "minimal-item")
	s.Require().NoError(err)
	s.Require().Empty(item.Comment)
	s.Require().Empty(item.Categories)
	s.Require().False(item.IsHidden)
}

func (s *CLITestSuite) TestAddUser() {
	out, err := s.execute("user", "add", "added-user", "--comment", "new user", "--labels", `{"role":"reader"}`)
	s.Require().NoError(err)
	s.Require().Contains(out, "RowAffected: 1")
	user, err := s.m.DataClient.GetUser(s.T().Context(), "added-user")
	s.Require().NoError(err)
	s.Require().Equal("new user", user.Comment)
	s.Require().Equal("reader", user.Labels.(map[string]any)["role"])

	_, err = s.execute("user", "add", "minimal-user")
	s.Require().NoError(err)
	user, err = s.m.DataClient.GetUser(s.T().Context(), "minimal-user")
	s.Require().NoError(err)
	s.Require().Empty(user.Comment)
}

func (s *CLITestSuite) TestDeleteItem() {
	for _, flags := range [][]string{nil, {"--yes=false"}} {
		_, err := s.execute(append([]string{"item", "delete", "item-1"}, flags...)...)
		s.Require().ErrorContains(err, "--yes")
		_, err = s.m.DataClient.GetItem(s.T().Context(), "item-1")
		s.Require().NoError(err)
	}
	out, err := s.execute("item", "delete", "item-1", "--yes")
	s.Require().NoError(err)
	s.Require().Contains(out, "RowAffected: 1")
	_, err = s.m.DataClient.GetItem(s.T().Context(), "item-1")
	s.Require().ErrorIs(err, storage.ErrNotFound)
}

func (s *CLITestSuite) TestDeleteUser() {
	for _, flags := range [][]string{nil, {"--yes=false"}} {
		_, err := s.execute(append([]string{"user", "delete", "alice"}, flags...)...)
		s.Require().ErrorContains(err, "--yes")
		_, err = s.m.DataClient.GetUser(s.T().Context(), "alice")
		s.Require().NoError(err)
	}
	out, err := s.execute("user", "delete", "alice", "--yes")
	s.Require().NoError(err)
	s.Require().Contains(out, "RowAffected: 1")
	_, err = s.m.DataClient.GetUser(s.T().Context(), "alice")
	s.Require().ErrorIs(err, storage.ErrNotFound)
}

func TestMutationAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != testAPIKey {
			t.Error("missing API key")
		}
		http.Error(w, "mutation failed", http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, entity := range []string{"item", "user"} {
		for _, action := range []string{"add", "delete"} {
			t.Run(entity+"/"+action, func(t *testing.T) {
				args := []string{entity, action, "id", "--endpoint", server.URL, "--api-key", testAPIKey}
				if action == "delete" {
					args = append(args, "--yes")
				}
				out, err := executeRawCommand(rootCmd, "", args...)
				require.ErrorContains(t, err, "API request failed")
				require.ErrorContains(t, err, "mutation failed")
				require.NotContains(t, out, "RowAffected:")
			})
		}
	}
}

func TestMutationValidation(t *testing.T) {
	for _, entity := range []string{"item", "user"} {
		t.Run(entity, func(t *testing.T) {
			for _, action := range []string{"add", "delete"} {
				for _, ids := range [][]string{nil, {"one", "two"}} {
					_, err := executeRawCommand(rootCmd, "", append([]string{entity, action}, ids...)...)
					require.ErrorContains(t, err, "accepts 1 arg(s)")
				}
			}
			for _, flags := range [][]string{nil, {"--yes=false"}} {
				_, err := executeRawCommand(rootCmd, "y\n", append([]string{entity, "delete", "id"}, flags...)...)
				require.ErrorContains(t, err, "--yes")
			}
			_, err := executeRawCommand(rootCmd, "", entity, "add", "id", "--labels", "{")
			require.ErrorContains(t, err, "invalid --labels")
		})
	}
	_, err := executeRawCommand(rootCmd, "", "item", "add", "id", "--timestamp", "invalid")
	require.ErrorContains(t, err, "invalid --timestamp")
}
