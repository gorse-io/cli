package internal

import (
	"fmt"
	"strings"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/gorse-io/gorse/storage"
	"github.com/gorse-io/gorse/storage/cache"
	"github.com/gorse-io/gorse/storage/data"
)

func (s *CLITestSuite) TestCategories() {
	s.Require().NoError(s.master.CacheClient.AddScores(s.T().Context(), cache.ItemCategories, "", []cache.Score{
		{Id: "news", Score: 2},
		{Id: "tech", Score: 1},
	}))
	out, err := s.Command("item", "categories")
	s.Require().NoError(err)
	s.EqualTable([][]string{{"INDEX", "VALUE"}, {"0", "news"}, {"1", "tech"}}, out)
}

func (s *CLITestSuite) TestItemImportExport() {
	_, err := s.Command("item", "add", "bulk-item-a", "--comment", "old comment")
	s.Require().NoError(err)
	testImportExport[gorse.Item](s, "item", "2 items imported\n", `{"ItemId":"bulk-item-a","IsHidden":true,"Categories":["news","tech"],"Timestamp":"2026-01-02T03:04:05Z","Labels":{"embedding":[0.1,0.2],"genre":"science"},"Comment":"物品 A"}
{"ItemId":"bulk-item-b","IsHidden":false,"Categories":[],"Timestamp":"2026-02-03T04:05:06Z","Labels":{},"Comment":"Item B"}
`)
}

func (s *CLITestSuite) TestItemListPagination() {
	ctx := s.T().Context()
	timestamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s.Require().NoError(s.master.DataClient.BatchInsertItems(ctx, []data.Item{
		{ItemId: "page-a", Comment: "first", Timestamp: timestamp},
		{ItemId: "page-b", Comment: "second", Timestamp: timestamp},
		{ItemId: "page-c", Comment: "third", Timestamp: timestamp},
	}))
	header := []string{"ITEM-ID", "COMMENT", "CATEGORIES", "IS-HIDDEN", "TIMESTAMP", "LABELS"}
	first := []string{"page-a", "first", "[]", "false", timestamp.Format(time.RFC3339), ""}
	second := []string{"page-b", "second", "[]", "false", timestamp.Format(time.RFC3339), ""}
	third := []string{"page-c", "third", "[]", "false", timestamp.Format(time.RFC3339), ""}
	cursor, _, err := s.master.DataClient.GetItems(ctx, "", 2, nil)
	s.Require().NoError(err)
	s.Require().NotEmpty(cursor)
	out, err := s.Command("item", "list", "-n", "2")
	s.Require().NoError(err)
	table, next, found := strings.Cut(out, "\nNext page: ")
	s.Require().True(found)
	s.EqualTable([][]string{header, first, second}, table)
	s.Equal(fmt.Sprintf("gorse-cli item list -n 2 --cursor '%s'\n", cursor), next)

	out, err = s.Command("item", "list", "-n", "2", "--cursor", cursor)
	s.Require().NoError(err)
	s.EqualTable([][]string{header, third}, out)
	for _, flags := range [][]string{nil, {"-n", "0"}, {"-n", "3"}} {
		out, err = s.Command(append([]string{"item", "list"}, flags...)...)
		s.Require().NoError(err)
		s.EqualTable([][]string{header, first, second, third}, out)
	}
	s.Require().NoError(s.master.DataClient.Purge())
	out, err = s.Command("item", "list")
	s.Require().NoError(err)
	s.EqualTable([][]string{header}, out)
}

func (s *CLITestSuite) TestItemUpdate() {
	ctx := s.T().Context()
	_, err := s.Command("item", "add", "updated-item", "--comment", "original comment",
		"--category", "news", "--category", "tech", "--is-hidden",
		"--timestamp", "2026-01-02T03:04:05Z", "--labels", `{"genre":"science","keep":"original"}`)
	s.Require().NoError(err)
	expected, err := s.master.DataClient.GetItem(ctx, "updated-item")
	s.Require().NoError(err)
	expected.UpdateAt = time.Time{}
	check := func() {
		s.T().Helper()
		actual, err := s.master.DataClient.GetItem(ctx, "updated-item")
		s.Require().NoError(err)
		actual.UpdateAt = time.Time{}
		s.Equal(expected, actual)
	}

	out, err := s.Command("item", "update", "updated-item", "--comment", "updated comment")
	s.Require().NoError(err)
	s.Equal("1 item updated\n", out)
	expected.Comment = "updated comment"
	check()
	_, err = s.Command("item", "update", "updated-item", "--is-hidden=false")
	s.Require().NoError(err)
	expected.IsHidden = false
	check()
	_, err = s.Command("item", "update", "updated-item", "--category", "books", "--category", "art")
	s.Require().NoError(err)
	expected.Categories = []string{"books", "art"}
	check()
	_, err = s.Command("item", "update", "updated-item", "--timestamp", "2026-02-03T04:05:06Z")
	s.Require().NoError(err)
	expected.Timestamp = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	check()
	_, err = s.Command("item", "update", "updated-item", "--labels", `{"genre":"fiction"}`)
	s.Require().NoError(err)
	expected.Labels = map[string]any{"genre": "fiction"}
	check()
	_, err = s.Command("item", "update", "updated-item", "--comment=")
	s.Require().NoError(err)
	expected.Comment = ""
	check()
	_, err = s.Command("item", "update", "updated-item", "--category=")
	s.Require().NoError(err)
	expected.Categories = []string{}
	check()
	_, err = s.Command("item", "update", "updated-item", "--labels", "{}")
	s.Require().NoError(err)
	expected.Labels = map[string]any{}
	check()
	_, err = s.Command("item", "update", "updated-item", "--timestamp", "0001-01-01T00:00:00Z")
	s.Require().NoError(err)
	expected.Timestamp = time.Time{}
	check()
	_, err = s.Command("item", "update", "updated-item", "--is-hidden", "--comment", "final comment", "--labels", "[]")
	s.Require().NoError(err)
	expected.IsHidden = true
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
		{[]string{"--timestamp", "invalid"}, "invalid --timestamp"},
		{[]string{"--timestamp="}, "invalid --timestamp"},
		{[]string{"--category=", "--category", "news"}, "invalid --category"},
		{[]string{"--labels", "true", "--comment", "must not change"}, "API request failed"},
	} {
		before, err := s.master.DataClient.GetItem(ctx, "updated-item")
		s.Require().NoError(err)
		out, err = s.Command(append([]string{"item", "update", "updated-item"}, tc.flags...)...)
		s.ErrorContains(err, tc.error)
		s.Empty(out)
		after, err := s.master.DataClient.GetItem(ctx, "updated-item")
		s.Require().NoError(err)
		s.Equal(before, after)
	}
	_, err = s.Command("item", "update")
	s.ErrorContains(err, "accepts 1 arg(s)")
}

func (s *CLITestSuite) TestItem() {
	out, err := s.Command("item", "add", "added-item", "--comment", "new item",
		"--category", "news", "--category", "tech", "--is-hidden",
		"--timestamp", "2026-01-02T03:04:05Z", "--labels", `{"genre":"science","embedding":[0.1,0.2]}`)
	s.Require().NoError(err)
	s.Equal("1 item added\n", out)
	item, err := s.master.DataClient.GetItem(s.T().Context(), "added-item")
	s.Require().NoError(err)
	s.Equal("new item", item.Comment)
	s.Equal([]string{"news", "tech"}, item.Categories)
	s.True(item.IsHidden)
	s.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), item.Timestamp)
	s.Equal("science", item.Labels.(map[string]any)["genre"])

	out, err = s.Command("item", "add", "minimal-item")
	s.Require().NoError(err)
	s.Equal("1 item added\n", out)
	item, err = s.master.DataClient.GetItem(s.T().Context(), "minimal-item")
	s.Require().NoError(err)
	s.Empty(item.Comment)
	s.Empty(item.Categories)
	s.False(item.IsHidden)

	out, err = s.Command("item", "get", "added-item")
	s.Require().NoError(err)
	s.Equal(`ItemId:     added-item
Comment:    new item
Categories: ["news","tech"]
IsHidden:   true
Timestamp:  2026-01-02T03:04:05Z
Labels:
{
	"embedding": [
		0.1,
		0.2
	],
	"genre": "science"
}
`, out)

	expected := [][]string{
		{"ITEM-ID", "COMMENT", "CATEGORIES", "IS-HIDDEN", "TIMESTAMP", "LABELS"},
		{"added-item", "new item", `["news","tech"]`, "true", "2026-01-02T03:04:05Z", `{"embedding":[0.1,0.2],"genre":"science"}`},
		{"minimal-item", "", "[]", "false", item.Timestamp.Format(time.RFC3339), "{}"},
	}
	out, err = s.Command("item", "list", "-n", "1")
	s.Require().NoError(err)
	table, _, _ := strings.Cut(out, "\nNext page: ")
	s.EqualTable(expected[:2], table)

	out, err = s.Command("item", "list")
	s.Require().NoError(err)
	s.EqualTable(expected, out)

	for _, flags := range [][]string{nil, {"--yes=false"}} {
		_, err = s.Command(append([]string{"item", "delete", "added-item"}, flags...)...)
		s.Require().ErrorContains(err, "--yes")
		_, err = s.master.DataClient.GetItem(s.T().Context(), "added-item")
		s.Require().NoError(err)
	}
	out, err = s.Command("item", "delete", "added-item", "--yes")
	s.Require().NoError(err)
	s.Equal("1 item deleted\n", out)
	_, err = s.master.DataClient.GetItem(s.T().Context(), "added-item")
	s.ErrorIs(err, storage.ErrNotFound)
}
