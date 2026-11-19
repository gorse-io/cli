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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/gorse-io/gorse/storage"
	"github.com/gorse-io/gorse/storage/data"
)

func (s *CLITestSuite) TestUserImportExport() {
	_, err := s.Command("user", "add", "bulk-user-a", "--comment", "old comment")
	s.Require().NoError(err)
	input := `{"UserId":"bulk-user-a","Labels":{"role":"reader"},"Comment":"用户 A"}
{"UserId":"bulk-user-b","Labels":["writer"],"Comment":"User B"}
`
	expected := decodeJSONLines[gorse.User](s.T(), input)
	dir := s.T().TempDir()
	inputPath := filepath.Join(dir, "import data.jsonl")
	outputPath := filepath.Join(dir, "export data.jsonl")
	s.Require().NoError(os.WriteFile(inputPath, []byte(input), 0o600))
	out, err := s.Command("user", "import", inputPath)
	s.Require().NoError(err)
	s.Equal("2 users imported\n", out)
	out, err = s.Command("user", "export", outputPath)
	s.Require().NoError(err)
	s.Contains(out, "Data exported to "+outputPath)
	content, err := os.ReadFile(outputPath)
	s.Require().NoError(err)
	s.ElementsMatch(expected, decodeJSONLines[gorse.User](s.T(), string(content)))

	defer s.root.SetIn(nil)
	for _, flags := range [][]string{{"-"}, nil} {
		s.Require().NoError(s.master.DataClient.Purge())
		out, err = s.Command("user", "export")
		s.Require().NoError(err)
		s.Empty(out)
		s.root.SetIn(strings.NewReader(string(content)))
		out, err = s.Command(append([]string{"user", "import"}, flags...)...)
		s.Require().NoError(err)
		s.Equal("2 users imported\n", out)
		out, err = s.Command(append([]string{"user", "export"}, flags...)...)
		s.Require().NoError(err)
		s.ElementsMatch(expected, decodeJSONLines[gorse.User](s.T(), out))
	}
}

func (s *CLITestSuite) TestUserPagination() {
	ctx := s.T().Context()
	s.Require().NoError(s.master.DataClient.BatchInsertUsers(ctx, []data.User{
		{UserId: "page-a", Comment: "first"},
		{UserId: "page-b", Comment: "second"},
		{UserId: "page-c", Comment: "third"},
	}))
	cursor, _, err := s.master.DataClient.GetUsers(ctx, "", 2)
	s.Require().NoError(err)
	s.Require().NotEmpty(cursor)
	out, err := s.Command("user", "list", "-n", "2")
	s.Require().NoError(err)
	table, next, found := strings.Cut(out, "\nNext page: ")
	s.Require().True(found)
	s.EqualTable([][]string{
		{"USER-ID", "LABELS", "COMMENT"},
		{"page-a", "", "first"},
		{"page-b", "", "second"},
	}, table)
	s.Equal(fmt.Sprintf("gorse-cli user list -n 2 --cursor %s\n", cursor), next)

	out, err = s.Command("user", "list", "-n", "2", "--cursor", cursor)
	s.Require().NoError(err)
	s.EqualTable([][]string{
		{"USER-ID", "LABELS", "COMMENT"},
		{"page-c", "", "third"},
	}, out)
	for _, flags := range [][]string{nil, {"-n", "0"}, {"-n", "3"}} {
		out, err = s.Command(append([]string{"user", "list"}, flags...)...)
		s.Require().NoError(err)
		s.EqualTable([][]string{
			{"USER-ID", "LABELS", "COMMENT"},
			{"page-a", "", "first"},
			{"page-b", "", "second"},
			{"page-c", "", "third"},
		}, out)
	}
	s.Require().NoError(s.master.DataClient.Purge())
	out, err = s.Command("user", "list")
	s.Require().NoError(err)
	s.EqualTable([][]string{
		{"USER-ID", "LABELS", "COMMENT"},
	}, out)
}

func (s *CLITestSuite) TestUserUpdate() {
	ctx := s.T().Context()
	_, err := s.Command("user", "add", "updated-user", "--comment", "original comment", "--labels", `{"role":"reader","keep":"original"}`)
	s.Require().NoError(err)
	expected, err := s.master.DataClient.GetUser(ctx, "updated-user")
	s.Require().NoError(err)
	expected.UpdateAt = time.Time{}
	check := func() {
		s.T().Helper()
		actual, err := s.master.DataClient.GetUser(ctx, "updated-user")
		s.Require().NoError(err)
		actual.UpdateAt = time.Time{}
		s.Equal(expected, actual)
	}

	out, err := s.Command("user", "update", "updated-user", "--comment", "updated comment")
	s.Require().NoError(err)
	s.Equal("1 user updated\n", out)
	expected.Comment = "updated comment"
	check()
	_, err = s.Command("user", "update", "updated-user", "--labels", `{"role":"writer"}`)
	s.Require().NoError(err)
	expected.Labels = map[string]any{"role": "writer"}
	check()
	_, err = s.Command("user", "update", "updated-user", "--comment=")
	s.Require().NoError(err)
	expected.Comment = ""
	check()
	_, err = s.Command("user", "update", "updated-user", "--labels", "{}")
	s.Require().NoError(err)
	expected.Labels = map[string]any{}
	check()
	_, err = s.Command("user", "update", "updated-user", "--comment", "final comment", "--labels", "[]")
	s.Require().NoError(err)
	expected.Comment = "final comment"
	expected.Labels = []any{}
	check()

	for _, tc := range []struct {
		flags []string
		error string
	}{
		{nil, "at least one update flag"},
		{[]string{"--labels", "invalid"}, "invalid --labels"},
		{[]string{"--labels", "null"}, "invalid --labels"},
		{[]string{"--labels", "true", "--comment", "must not change"}, "API request failed"},
	} {
		before, err := s.master.DataClient.GetUser(ctx, "updated-user")
		s.Require().NoError(err)
		out, err = s.Command(append([]string{"user", "update", "updated-user"}, tc.flags...)...)
		s.ErrorContains(err, tc.error)
		s.Empty(out)
		after, err := s.master.DataClient.GetUser(ctx, "updated-user")
		s.Require().NoError(err)
		s.Equal(before, after)
	}
	_, err = s.Command("user", "update")
	s.ErrorContains(err, "accepts 1 arg(s)")
}

func (s *CLITestSuite) TestUser() {
	out, err := s.Command("user", "add", "added-user", "--comment", "new user", "--labels", `{"role":"reader"}`)
	s.Require().NoError(err)
	s.Equal("1 user added\n", out)
	user, err := s.master.DataClient.GetUser(s.T().Context(), "added-user")
	s.Require().NoError(err)
	s.Equal("new user", user.Comment)
	s.Equal("reader", user.Labels.(map[string]any)["role"])

	out, err = s.Command("user", "add", "minimal-user")
	s.Require().NoError(err)
	s.Equal("1 user added\n", out)
	user, err = s.master.DataClient.GetUser(s.T().Context(), "minimal-user")
	s.Require().NoError(err)
	s.Empty(user.Comment)

	out, err = s.Command("user", "get", "added-user")
	s.Require().NoError(err)
	s.Equal(`UserId:  added-user
Comment: new user
Labels:
{
	"role": "reader"
}
`, out)

	expected := [][]string{
		{"USER-ID", "LABELS", "COMMENT"},
		{"added-user", `{"role":"reader"}`, "new user"},
		{"minimal-user", "{}", ""},
	}
	out, err = s.Command("user", "list", "-n", "1")
	s.Require().NoError(err)
	table, _, _ := strings.Cut(out, "\nNext page: ")
	s.EqualTable(expected[:2], table)

	out, err = s.Command("user", "list")
	s.Require().NoError(err)
	s.EqualTable(expected, out)

	for _, flags := range [][]string{nil, {"--yes=false"}} {
		_, err = s.Command(append([]string{"user", "delete", "added-user"}, flags...)...)
		s.Require().ErrorContains(err, "--yes")
		_, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
		s.Require().NoError(err)
	}
	out, err = s.Command("user", "delete", "added-user", "--yes")
	s.Require().NoError(err)
	s.Equal("1 user deleted\n", out)
	_, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
	s.ErrorIs(err, storage.ErrNotFound)
}
