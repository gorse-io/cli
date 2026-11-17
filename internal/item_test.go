package internal

import (
	"time"

	"github.com/gorse-io/gorse/storage"
	"github.com/gorse-io/gorse/storage/cache"
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

func (s *CLITestSuite) TestItem() {
	out, err := s.Command("item", "add", "added-item", "--comment", "new item",
		"--category", "news", "--category", "tech", "--is-hidden",
		"--timestamp", "2026-01-02T03:04:05Z", "--labels", `{"genre":"science","embedding":[0.1,0.2]}`)
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1")
	item, err := s.master.DataClient.GetItem(s.T().Context(), "added-item")
	s.Require().NoError(err)
	s.Equal("new item", item.Comment)
	s.Equal([]string{"news", "tech"}, item.Categories)
	s.True(item.IsHidden)
	s.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), item.Timestamp)
	s.Equal("science", item.Labels.(map[string]any)["genre"])

	out, err = s.Command("item", "add", "minimal-item")
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1")
	item, err = s.master.DataClient.GetItem(s.T().Context(), "minimal-item")
	s.Require().NoError(err)
	s.Empty(item.Comment)
	s.Empty(item.Categories)
	s.False(item.IsHidden)

	out, err = s.Command("item", "get", "added-item")
	s.Require().NoError(err)
	s.Contains(out, "ItemId: added-item\n")
	s.Contains(out, "Comment: new item\n")
	s.Contains(out, `Categories: ["news","tech"]`+"\n")
	s.Contains(out, "IsHidden: true\n")
	s.Contains(out, "Timestamp: 2026-01-02T03:04:05Z\n")
	s.Contains(out, `"genre": "science"`)

	expected := [][]string{
		{"ITEM-ID", "COMMENT", "CATEGORIES", "IS-HIDDEN", "TIMESTAMP", "LABELS"},
		{"added-item", "new item", `["news","tech"]`, "true", "2026-01-02T03:04:05Z", `{"embedding":[0.1,0.2],"genre":"science"}`},
		{"minimal-item", "", "[]", "false", item.Timestamp.Format(time.RFC3339), "{}"},
	}
	out, err = s.Command("item", "list", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable(expected[:2], out)

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
	s.Contains(out, "RowAffected: 1")
	_, err = s.master.DataClient.GetItem(s.T().Context(), "added-item")
	s.ErrorIs(err, storage.ErrNotFound)
}
