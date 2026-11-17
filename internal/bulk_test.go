package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/stretchr/testify/require"
)

func decodeJSONL[T any](t *testing.T, input string) []T {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(input))
	var records []T
	for {
		var record T
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		require.NoError(t, err)
		records = append(records, record)
	}
}

func testImportExport[T any](s *CLITestSuite, resource, input string) {
	s.T().Helper()
	expected := decodeJSONL[T](s.T(), input)
	dir := s.T().TempDir()
	inputPath := filepath.Join(dir, "import data.jsonl")
	outputPath := filepath.Join(dir, "export data.jsonl")
	s.Require().NoError(os.WriteFile(inputPath, []byte(input), 0o600))
	out, err := s.Command(resource, "import", inputPath)
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 2")
	out, err = s.Command(resource, "export", outputPath)
	s.Require().NoError(err)
	s.Contains(out, "Data exported to "+outputPath)
	content, err := os.ReadFile(outputPath)
	s.Require().NoError(err)
	s.ElementsMatch(expected, decodeJSONL[T](s.T(), string(content)))

	defer s.root.SetIn(nil)
	for _, flags := range [][]string{{"-"}, nil} {
		s.Require().NoError(s.master.DataClient.Purge())
		out, err = s.Command(resource, "export")
		s.Require().NoError(err)
		s.Empty(out)
		s.root.SetIn(strings.NewReader(string(content)))
		out, err = s.Command(append([]string{resource, "import"}, flags...)...)
		s.Require().NoError(err)
		s.Contains(out, "RowAffected: 2")
		out, err = s.Command(append([]string{resource, "export"}, flags...)...)
		s.Require().NoError(err)
		s.ElementsMatch(expected, decodeJSONL[T](s.T(), out))
	}
}

func (s *CLITestSuite) TestImportExportErrors() {
	defer s.root.SetIn(nil)
	for _, resource := range []string{"user", "item", "feedback"} {
		path := filepath.Join(s.T().TempDir(), "missing.jsonl")
		out, err := s.Command(resource, "import", path)
		s.ErrorContains(err, "failed to open import file")
		s.Empty(out)
		s.Require().NoError(os.WriteFile(path, []byte("keep existing file"), 0o600))
		out, err = s.Command(resource, "export", path, "--api-key", "invalid-key")
		s.ErrorContains(err, "status=401")
		s.Empty(out)
		content, err := os.ReadFile(path)
		s.Require().NoError(err)
		s.Equal("keep existing file", string(content))
		s.root.SetIn(strings.NewReader(`{"Labels":true}`))
		out, err = s.Command(resource, "import")
		s.ErrorContains(err, "API request failed")
		s.Empty(out)
		s.root.SetIn(strings.NewReader(""))
		out, err = s.Command(resource, "import")
		s.Require().NoError(err)
		s.Contains(out, "RowAffected: 0")
		_, err = s.Command(resource, "export", "one", "two")
		s.ErrorContains(err, "accepts at most 1 arg(s)")
	}
}

func (s *CLITestSuite) TestBulkTransport() {
	// The CLI forwards input verbatim, even when it is not valid JSONL.
	input := "not JSONL\x00\n {unfinished"
	defer s.root.SetIn(nil)
	for _, resource := range []struct{ command, path string }{
		{"user", "users"}, {"item", "items"}, {"feedback", "feedback"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("transport-key", r.Header.Get("X-API-Key"))
			s.Equal("/prefix/api/bulk/"+resource.path, r.URL.Path)
			switch r.Method {
			case http.MethodGet:
				fmt.Fprint(w, input)
			case http.MethodPost:
				file, _, err := r.FormFile("file")
				if !s.NoError(err) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer file.Close()
				defer r.MultipartForm.RemoveAll()
				content, err := io.ReadAll(file)
				s.NoError(err)
				s.Equal(input, string(content))
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintln(w, `{"RowAffected":1}`)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		defer server.Close()
		s.root.SetIn(strings.NewReader(input))
		out, err := s.Command(resource.command, "import", "--endpoint", server.URL+"/prefix/", "--api-key", "transport-key")
		s.Require().NoError(err)
		s.Contains(out, "RowAffected: 1")
		out, err = s.Command(resource.command, "export", "--endpoint", server.URL+"/prefix/", "--api-key", "transport-key")
		s.Require().NoError(err)
		s.Equal(input, out)
	}
}

func (s *CLITestSuite) TestExportInterrupted() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, `{"UserId":"incomplete`)
	}))
	defer server.Close()
	dir := s.T().TempDir()
	path := filepath.Join(dir, "users.jsonl")
	s.Require().NoError(os.WriteFile(path, []byte("keep existing file"), 0o600))
	out, err := s.Command("user", "export", path, "--endpoint", server.URL)
	s.ErrorContains(err, "failed to export data")
	s.Empty(out)
	content, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal("keep existing file", string(content))
	entries, err := os.ReadDir(dir)
	s.Require().NoError(err)
	s.Len(entries, 1)
}

func (s *CLITestSuite) TestBulkRedirect() {
	forwarded := make(chan bool, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded <- true
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"RowAffected":1}`)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	s.root.SetIn(strings.NewReader(`{"UserId":"redirect-user"}`))
	defer s.root.SetIn(nil)
	out, err := s.Command("user", "import", "--endpoint", redirect.URL)
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1")
	select {
	case <-forwarded:
	default:
		s.Fail("the SDK's default redirect behavior should be retained")
	}
}

func (s *CLITestSuite) TestUserImportLarge() {
	var input strings.Builder
	for i := 0; i < 1001; i++ {
		fmt.Fprintf(&input, "{\"UserId\":\"batch-%04d\",\"Comment\":\"bulk user\"}\n", i)
	}
	s.root.SetIn(strings.NewReader(input.String()))
	defer s.root.SetIn(nil)
	out, err := s.Command("user", "import")
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1001")
	out, err = s.Command("user", "export")
	s.Require().NoError(err)
	s.Len(decodeJSONL[gorse.User](s.T(), out), 1001)
}
