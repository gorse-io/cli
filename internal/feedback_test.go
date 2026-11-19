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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorse-io/gorse/server"
	"github.com/gorse-io/gorse/storage/data"
)

func (s *CLITestSuite) TestFeedbackImportExport() {
	_, err := s.Command("feedback", "add", "click", "bulk-user", "bulk-item", "--value", "1", "--timestamp", "2026-01-01T00:00:00Z")
	s.Require().NoError(err)
	input := `{"FeedbackType":"click","UserId":"bulk-user","ItemId":"bulk-item","Value":2.5,"Timestamp":"2026-01-02T03:04:05Z","Labels":{"source":"web"},"Comment":"反馈 A"}
{"FeedbackType":"read","UserId":"bulk-user","ItemId":"bulk-item","Value":0,"Timestamp":"2026-02-03T04:05:06Z","Labels":[],"Comment":"Feedback B"}
`
	expected := decodeJSONLines[server.Feedback](s.T(), input)
	dir := s.T().TempDir()
	inputPath := filepath.Join(dir, "import data.jsonl")
	outputPath := filepath.Join(dir, "export data.jsonl")
	s.Require().NoError(os.WriteFile(inputPath, []byte(input), 0o600))
	out, err := s.Command("feedback", "import", inputPath)
	s.Require().NoError(err)
	s.Equal("2 feedback imported\n", out)
	out, err = s.Command("feedback", "export", outputPath)
	s.Require().NoError(err)
	s.Contains(out, "Data exported to "+outputPath)
	content, err := os.ReadFile(outputPath)
	s.Require().NoError(err)
	s.ElementsMatch(expected, decodeJSONLines[server.Feedback](s.T(), string(content)))

	defer s.root.SetIn(nil)
	for _, flags := range [][]string{{"-"}, nil} {
		s.Require().NoError(s.master.DataClient.Purge())
		out, err = s.Command("feedback", "export")
		s.Require().NoError(err)
		s.Empty(out)
		s.root.SetIn(strings.NewReader(string(content)))
		out, err = s.Command(append([]string{"feedback", "import"}, flags...)...)
		s.Require().NoError(err)
		s.Equal("2 feedback imported\n", out)
		out, err = s.Command(append([]string{"feedback", "export"}, flags...)...)
		s.Require().NoError(err)
		s.ElementsMatch(expected, decodeJSONLines[server.Feedback](s.T(), out))
	}
}

func (s *CLITestSuite) TestFeedback() {
	ctx := s.T().Context()
	s.Require().NoError(s.master.DataClient.BatchInsertUsers(ctx, []data.User{{UserId: "feedback-user"}}))
	s.Require().NoError(s.master.DataClient.BatchInsertItems(ctx, []data.Item{{ItemId: "feedback-item"}}))

	out, err := s.Command("feedback", "add", "click", "feedback-user", "feedback-item",
		"--value", "2.5", "--timestamp", "2026-01-02T03:04:05Z")
	s.Require().NoError(err)
	s.Equal("1 feedback added\n", out)
	records, err := s.master.DataClient.GetUserItemFeedback(ctx, "feedback-user", "feedback-item", "click")
	s.Require().NoError(err)
	s.Require().Len(records, 1)
	click := records[0]
	s.Equal(2.5, click.Value)
	s.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), click.Timestamp)

	before := time.Now().Add(-time.Second)
	out, err = s.Command("feedback", "add", "view", "feedback-user", "feedback-item")
	s.Require().NoError(err)
	s.Equal("1 feedback added\n", out)
	records, err = s.master.DataClient.GetUserItemFeedback(ctx, "feedback-user", "feedback-item", "view")
	s.Require().NoError(err)
	s.Require().Len(records, 1)
	view := records[0]
	s.Equal(1.0, view.Value)
	s.True(view.Timestamp.After(before))
	s.False(view.Timestamp.After(time.Now()))
	_, err = s.Command("feedback", "delete", "view", "feedback-user", "feedback-item", "--yes")
	s.Require().NoError(err)

	_, err = s.Command("feedback", "add", "read", "feedback-user", "feedback-item", "--timestamp", "2026-02-03T04:05:06Z")
	s.Require().NoError(err)
	records, err = s.master.DataClient.GetUserItemFeedback(ctx, "feedback-user", "feedback-item", "read")
	s.Require().NoError(err)
	s.Require().Len(records, 1)
	read := records[0]
	s.Equal(1.0, read.Value)

	header := []string{"TIMESTAMP", "FEEDBACK-TYPE", "USER-ID", "ITEM-ID", "VALUE", "LABELS", "UPDATED", "COMMENT"}
	clickRow := []string{click.Timestamp.String(), "click", "feedback-user", "feedback-item", "2.5", "null", click.Updated.String(), ""}
	readRow := []string{read.Timestamp.String(), "read", "feedback-user", "feedback-item", "1", "null", read.Updated.String(), ""}
	for _, flags := range [][]string{
		nil,
		{"--user", "feedback-user"},
		{"--item", "feedback-item"},
		{"--user", "feedback-user", "--item", "feedback-item"},
	} {
		out, err = s.Command(append([]string{"feedback", "list"}, flags...)...)
		s.Require().NoError(err)
		s.EqualTable([][]string{
			header,
			readRow,
			clickRow,
		}, out)
		out, err = s.Command(append(append([]string{"feedback", "list", "--type", "click"}, flags...), "-n", "1")...)
		s.Require().NoError(err)
		s.EqualTable([][]string{
			header,
			clickRow,
		}, out)
	}
	out, err = s.Command("feedback", "list", "--user", "feedback-user", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable([][]string{
		header,
		readRow,
	}, out)

	_, err = s.Command("feedback", "add", "click", "feedback-user", "feedback-item", "--timestamp", "invalid")
	s.ErrorContains(err, "invalid --timestamp")
	for _, action := range []string{"add", "delete"} {
		_, err = s.Command("feedback", action)
		s.ErrorContains(err, "accepts 3 arg(s)")
	}
	for _, flags := range [][]string{nil, {"--yes=false"}} {
		_, err = s.Command(append([]string{"feedback", "delete", "click", "feedback-user", "feedback-item"}, flags...)...)
		s.ErrorContains(err, "--yes")
		records, err = s.master.DataClient.GetUserItemFeedback(ctx, "feedback-user", "feedback-item")
		s.Require().NoError(err)
		s.Len(records, 2)
	}
	out, err = s.Command("feedback", "delete", "click", "feedback-user", "feedback-item", "--yes")
	s.Require().NoError(err)
	s.Equal("1 feedback deleted\n", out)
	records, err = s.master.DataClient.GetUserItemFeedback(ctx, "feedback-user", "feedback-item")
	s.Require().NoError(err)
	s.Require().Len(records, 1)
	s.Equal("read", records[0].FeedbackType)

	out, err = s.Command("feedback", "delete", "click", "feedback-user", "feedback-item", "--yes")
	s.Require().NoError(err)
	s.Equal("0 feedback deleted\n", out)
}
