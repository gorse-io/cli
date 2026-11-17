package internal

import (
	"strings"
	"time"

	"github.com/gorse-io/gorse/storage"
)

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

	out, err = s.Command("item", "list", "-n", "1")
	s.Require().NoError(err)
	s.Len(strings.Split(strings.TrimSpace(out), "\n"), 2)
	s.Regexp(`(?m)^ITEM-ID\s+COMMENT\s+CATEGORIES\s+IS-HIDDEN\s+TIMESTAMP\s+LABELS$`, out)

	out, err = s.Command("item", "list")
	s.Require().NoError(err)
	s.Len(strings.Split(strings.TrimSpace(out), "\n"), 3)
	s.Regexp(`(?m)^added-item\s+new item\s+\["news","tech"\]\s+true\s+2026-01-02T03:04:05Z\s+\{"embedding":\[0.1,0.2\],"genre":"science"\}$`, out)
	s.Regexp(`(?m)^minimal-item\s+\[\]\s+false\s+`, out)

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
