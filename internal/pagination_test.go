package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestPrintNextPage(t *testing.T) {
	root := &cobra.Command{Use: "gorse-cli"}
	user := &cobra.Command{Use: "user"}
	cmd := &cobra.Command{Use: "list"}
	root.AddCommand(user)
	user.AddCommand(cmd)
	cmd.Flags().String("application", "", "")
	cmd.Flags().String("endpoint", "", "")
	cmd.Flags().String("api-key", "", "")
	var output bytes.Buffer
	cmd.SetOut(&output)
	printNextPage(cmd, "", 10)
	require.Empty(t, output.String())
	printNextPage(cmd, "abc+/=", 10)
	require.Equal(t, "\nNext page: gorse-cli user list -n 10 --cursor 'abc+/='\n", output.String())

	output.Reset()
	require.NoError(t, cmd.Flags().Set("application", "staging west"))
	require.NoError(t, cmd.Flags().Set("endpoint", "https://example.test/prefix"))
	require.NoError(t, cmd.Flags().Set("api-key", "private-test-key"))
	printNextPage(cmd, "next' $(echo unsafe)", 2)
	require.Equal(t, "\nNext page: gorse-cli user list -n 2 --cursor 'next'\"'\"' $(echo unsafe)' --application 'staging west' --endpoint 'https://example.test/prefix' --api-key '<api-key>'\n", output.String())
	require.NotContains(t, output.String(), "private-test-key")
}

func (s *CLITestSuite) TestListCursorTransport() {
	for _, resource := range []struct{ command, field string }{{"user", "Users"}, {"item", "Items"}} {
		cursor := "opaque +/%&n=999=next"
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			s.Equal(http.MethodGet, r.Method)
			s.Equal("/prefix/api/"+strings.ToLower(resource.field), r.URL.Path)
			s.Equal("pagination-key", r.Header.Get("X-API-Key"))
			s.Equal("5", r.URL.Query().Get("n"))
			s.Equal(cursor, r.URL.Query().Get("cursor"))
			s.Len(r.URL.Query(), 2)
			w.Header().Set("Content-Type", "application/json")
			s.NoError(json.NewEncoder(w).Encode(map[string]any{"Cursor": "next' token", resource.field: []any{}}))
		}))
		defer server.Close()
		out, err := s.Command(resource.command, "list", "-n", "5", "--cursor", cursor,
			"--application", "staging west", "--endpoint", server.URL+"/prefix", "--api-key", "pagination-key")
		s.Require().NoError(err)
		s.Equal(int32(1), requests.Load(), "only fetch the requested page")
		_, next, found := strings.Cut(out, "\nNext page: ")
		s.Require().True(found)
		s.Equal(fmt.Sprintf("gorse-cli %s list -n 5 --cursor 'next'\"'\"' token' --application 'staging west' --endpoint '%s/prefix' --api-key '<api-key>'\n", resource.command, server.URL), next)
		s.NotContains(out, "pagination-key")
	}
}

func (s *CLITestSuite) TestItemSearchCursor() {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		s.Equal("/api/items", r.URL.Path)
		s.Equal("two words", r.URL.Query().Get("q"))
		s.Equal("2", r.URL.Query().Get("n"))
		s.Empty(r.URL.Query().Get("cursor"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"Cursor":"not-a-list-page","Items":[]}`)
	}))
	defer server.Close()
	out, err := s.Command("item", "list", "two", "words", "-n", "2", "--endpoint", server.URL)
	s.Require().NoError(err)
	s.NotContains(out, "Next page:")
	out, err = s.Command("item", "list", "two", "words", "--cursor", "cursor", "--endpoint", server.URL)
	s.ErrorContains(err, "--cursor is not supported when searching items")
	s.Empty(out)
	s.Equal(int32(1), requests.Load(), "reject search cursors without sending a request")
}

func (s *CLITestSuite) TestListCursorErrors() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, "invalid cursor")
	}))
	defer server.Close()
	for _, resource := range []string{"user", "item"} {
		out, err := s.Command(resource, "list", "--cursor", "invalid", "--endpoint", server.URL)
		s.ErrorContains(err, "API request failed")
		s.Empty(out)
	}
}
