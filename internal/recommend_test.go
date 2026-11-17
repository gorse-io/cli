package internal

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gorse-io/gorse/storage/cache"
	"github.com/gorse-io/gorse/storage/data"
	"github.com/gorse-io/gorse/storage/vectors"
)

func (s *CLITestSuite) TestRecommendItemToItem() {
	ctx := s.T().Context()
	timestamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Require().NoError(s.master.DataClient.BatchInsertItems(ctx, []data.Item{
		{ItemId: "source", Timestamp: timestamp},
		{ItemId: "item-a", Comment: "close item", Categories: []string{"news"}, Timestamp: timestamp},
		{ItemId: "item-b", Comment: "other item", Categories: []string{"tech"}, Timestamp: timestamp},
		{ItemId: "hidden", IsHidden: true, Timestamp: timestamp},
	}))
	collection := vectors.ItemToItemCollection("second")
	s.Require().NoError(s.master.VectorClient.AddCollection(ctx, collection, 2, vectors.Dot, vectors.VectorConfig{}))
	s.Require().NoError(s.master.VectorClient.AddVectors(ctx, collection, []vectors.Vector{
		{Id: "source", Values: []float32{1, 0}},
		{Id: "item-a", Values: []float32{2, 0}, Categories: []string{"news"}},
		{Id: "item-b", Values: []float32{1, 1}, Categories: []string{"tech"}},
		{Id: "hidden", Values: []float32{3, 0}, IsHidden: true},
	}))
	s.Require().NoError(s.master.VectorClient.Optimize(ctx, collection))
	header := []string{"ITEM-ID", "COMMENT", "CATEGORIES", "IS-HIDDEN", "TIMESTAMP", "LABELS", "SCORE"}
	first := []string{"item-a", "close item", `["news"]`, "false", "2026-01-01T00:00:00Z", "", "1"}
	second := []string{"item-b", "other item", `["tech"]`, "false", "2026-01-01T00:00:00Z", "", "0.5"}
	out, err := s.Command("recommend", "item-to-item", "second", "source", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, first}, out)
	out, err = s.Command("recommend", "item-to-item", "second", "source", "--category", "tech")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, second}, out)
	out, err = s.Command("recommend", "item-to-item", "second", "source")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, first, second}, out)
	out, err = s.Command("recommend", "item-to-item", "second", "missing")
	s.Require().NoError(err)
	s.EqualTable([][]string{header}, out)
}

func (s *CLITestSuite) TestRecommendUserToUser() {
	ctx := s.T().Context()
	s.Require().NoError(s.master.DataClient.BatchInsertUsers(ctx, []data.User{
		{UserId: "source"},
		{UserId: "user-a", Comment: "close user", Labels: map[string]any{"role": "reader"}},
		{UserId: "user-b", Comment: "other user", Labels: map[string]any{"role": "writer"}},
	}))
	collection := vectors.UserToUserCollection("similar")
	s.Require().NoError(s.master.VectorClient.AddCollection(ctx, collection, 2, vectors.Euclidean, vectors.VectorConfig{}))
	s.Require().NoError(s.master.VectorClient.AddVectors(ctx, collection, []vectors.Vector{
		{Id: "source", Values: []float32{0, 0}},
		{Id: "user-a", Values: []float32{1, 0}},
		{Id: "user-b", Values: []float32{2, 0}},
	}))
	s.Require().NoError(s.master.VectorClient.Optimize(ctx, collection))
	header := []string{"USER-ID", "COMMENT", "LABELS", "SCORE"}
	first := []string{"user-a", "close user", `{"role":"reader"}`, "0.5"}
	second := []string{"user-b", "other user", `{"role":"writer"}`, "0.2"}
	out, err := s.Command("recommend", "user-to-user", "similar", "source", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, first}, out)
	out, err = s.Command("recommend", "user-to-user", "similar", "source")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, first, second}, out)
	out, err = s.Command("recommend", "user-to-user", "similar", "missing")
	s.Require().NoError(err)
	s.EqualTable([][]string{header}, out)
}

func (s *CLITestSuite) TestRecommend() {
	ctx := s.T().Context()
	s.Require().NoError(s.master.DataClient.BatchInsertUsers(ctx, []data.User{{UserId: "recommend-user"}}))
	items := make([]data.Item, 0, 13)
	header := []string{"ITEM-ID", "COMMENT", "CATEGORIES", "IS-HIDDEN", "TIMESTAMP", "LABELS", "SCORE"}
	rows := make(map[int][]string)
	for i := 1; i <= 13; i++ {
		timestamp := time.Date(2026, 1, i, 0, 0, 0, 0, time.UTC)
		categories := []string{"news"}
		categoryCell := `["news"]`
		if i%2 == 0 {
			categories = append(categories, "tech")
			categoryCell = `["news","tech"]`
		}
		item := data.Item{ItemId: fmt.Sprintf("item-%02d", i), Comment: fmt.Sprintf("Recommended item %d", i),
			Categories: categories, IsHidden: i == 13, Timestamp: timestamp, Labels: map[string]any{"group": "test"}}
		items = append(items, item)
		rows[i] = []string{item.ItemId, item.Comment, categoryCell, "false", timestamp.Format(time.RFC3339),
			`{"group":"test"}`, strconv.FormatInt(timestamp.Unix(), 10)}
	}
	s.Require().NoError(s.master.DataClient.BatchInsertItems(ctx, items))
	// The newest visible item has already been read and must not be recommended.
	s.Require().NoError(s.master.DataClient.BatchInsertFeedback(ctx, []data.Feedback{{
		FeedbackType: "read", UserId: "recommend-user", ItemId: "item-12", Value: 1, Timestamp: items[11].Timestamp,
	}}, false, false, true))

	out, err := s.Command("recommend", "item-to-user", "recommend-user", "-n", "2")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, rows[11], rows[10]}, out)
	out, err = s.Command("recommend", "item-to-user", "recommend-user", "--category", "news", "--category", "tech")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, rows[10], rows[8], rows[6], rows[4], rows[2]}, out)
	for _, flags := range [][]string{nil, {"-n", "0"}} {
		out, err = s.Command(append([]string{"recommend", "item-to-user", "recommend-user"}, flags...)...)
		s.Require().NoError(err)
		expected := [][]string{header}
		for i := 11; i >= 2; i-- {
			expected = append(expected, rows[i])
		}
		s.EqualTable(expected, out)
	}
	out, err = s.Command("recommend", "item-to-user", "recommend-user", "--category", "missing")
	s.Require().NoError(err)
	s.EqualTable([][]string{header}, out)
	out, err = s.Command("recommend", "item-to-user", "recommend-user", "latest", "-n", "2")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, rows[11], rows[10]}, out)

	s.Require().NoError(s.master.CacheClient.AddScores(ctx, cache.NonPersonalized, "popular", []cache.Score{
		{Id: "item-01", Score: 20, Categories: []string{"", "news"}},
		{Id: "item-02", Score: 10, Categories: []string{"", "news", "tech"}},
	}))
	s.Require().NoError(s.master.CacheClient.Set(ctx, cache.String(cache.Key(cache.NonPersonalizedDigest, "popular"), "test-digest")))
	popularFirst := append([]string(nil), rows[1]...)
	popularFirst[len(popularFirst)-1] = "20"
	popularSecond := append([]string(nil), rows[2]...)
	popularSecond[len(popularSecond)-1] = "10"
	out, err = s.Command("recommend", "item-to-user", "recommend-user", "non-personalized", "popular")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, popularFirst, popularSecond}, out)
	out, err = s.Command("recommend", "item-to-user", "recommend-user", "non-personalized", "popular", "--category", "tech", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, popularSecond}, out)

	// Global latest recommendations do not exclude this user's read items.
	out, err = s.Command("recommend", "latest", "-n", "2")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, rows[12], rows[11]}, out)
	out, err = s.Command("recommend", "latest", "--category", "news", "--category", "tech", "-n", "2")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, rows[12], rows[10]}, out)
	out, err = s.Command("recommend", "latest", "--category", "missing")
	s.Require().NoError(err)
	s.EqualTable([][]string{header}, out)

	s.Require().NoError(s.master.DataClient.BatchInsertFeedback(ctx, []data.Feedback{{
		FeedbackType: "read", UserId: "recommend-user", ItemId: "item-01", Value: 1, Timestamp: items[0].Timestamp,
	}}, false, false, true))
	out, err = s.Command("recommend", "non-personalized", "popular", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, popularFirst}, out)
	out, err = s.Command("recommend", "non-personalized", "popular", "--category", "tech", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, popularSecond}, out)
	out, err = s.Command("recommend", "non-personalized", "popular", "--user-id", "recommend-user", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, popularSecond}, out)
	out, err = s.Command("recommend", "non-personalized", "popular")
	s.Require().NoError(err)
	s.EqualTable([][]string{header, popularFirst, popularSecond}, out)
	_, err = s.Command("recommend", "item-to-user")
	s.ErrorContains(err, "accepts between 1 and 3 arg(s), received 0")
	_, err = s.Command("recommend", "item-to-user", "recommend-user", "latest", "unused", "extra")
	s.ErrorContains(err, "accepts between 1 and 3 arg(s), received 4")
}
