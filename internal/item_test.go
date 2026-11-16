package internal

import (
	"time"

	"github.com/gorse-io/gorse/storage"
	"github.com/spf13/pflag"
)

func (s *CLITestSuite) TestItemAddDelete() {
	defer func() {
		for _, cmd := range []*pflag.FlagSet{addItemCmd.Flags(), deleteItemCmd.Flags()} {
			cmd.VisitAll(func(flag *pflag.Flag) {
				if value, ok := flag.Value.(pflag.SliceValue); ok {
					s.NoError(value.Replace(nil))
				} else {
					s.NoError(flag.Value.Set(flag.DefValue))
				}
				flag.Changed = false
			})
		}
	}()

	s.Contains(s.Command("item", "add", "minimal-item"), "RowAffected: 1")
	item, err := s.master.DataClient.GetItem(s.T().Context(), "minimal-item")
	s.Require().NoError(err)
	s.Empty(item.Comment)
	s.Empty(item.Categories)
	s.False(item.IsHidden)

	out := s.Command("item", "add", "added-item", "--comment", "new item",
		"--category", "news", "--category", "tech", "--is-hidden",
		"--timestamp", "2026-01-02T03:04:05Z", "--labels", `{"genre":"science","embedding":[0.1,0.2]}`)
	s.Contains(out, "RowAffected: 1")
	item, err = s.master.DataClient.GetItem(s.T().Context(), "added-item")
	s.Require().NoError(err)
	s.Equal("new item", item.Comment)
	s.Equal([]string{"news", "tech"}, item.Categories)
	s.True(item.IsHidden)
	s.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), item.Timestamp)
	s.Equal("science", item.Labels.(map[string]any)["genre"])

	for _, flags := range [][]string{nil, {"--yes=false"}} {
		s.root.SetArgs(append([]string{"item", "delete", "added-item"}, flags...))
		s.Require().ErrorContains(s.root.Execute(), "--yes")
		_, err = s.master.DataClient.GetItem(s.T().Context(), "added-item")
		s.Require().NoError(err)
	}
	s.Contains(s.Command("item", "delete", "added-item", "--yes"), "RowAffected: 1")
	_, err = s.master.DataClient.GetItem(s.T().Context(), "added-item")
	s.ErrorIs(err, storage.ErrNotFound)
}
