package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/gorse-io/gorse/common/log"
	"github.com/gorse-io/gorse/config"
	"github.com/gorse-io/gorse/master"
	"github.com/gorse-io/gorse/storage/cache"
	"github.com/gorse-io/gorse/storage/data"
	"github.com/gorse-io/gorse/storage/vectors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CLITestSuite struct {
	suite.Suite
	root   *cobra.Command
	master *master.Master
	done   chan struct{}
}

func decodeJSONLines[T any](t *testing.T, input string) []T {
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

func TestCLI(t *testing.T) {
	suite.Run(t, new(CLITestSuite))
}

func (s *CLITestSuite) SetupSuite() {
	t := s.T()
	log.SetTestLogger(t)
	tempDir := t.TempDir()
	cfg := config.GetDefaultConfig()
	cfg.Database.DataStore = "sqlite://" + filepath.Join(tempDir, "data.db")
	cfg.Database.CacheStore = "sqlite://" + filepath.Join(tempDir, "cache.db")
	cfg.Database.VectorStore = "xvec://" + filepath.Join(tempDir, "vectors")
	cfg.Blob.URI = filepath.Join(tempDir, "blob")
	cfg.Master.Host = "127.0.0.1"
	cfg.Master.HttpHost = "127.0.0.1"
	cfg.Master.AdminAPIKey = "test-key"
	cfg.Master.DashboardUserName = "admin"
	cfg.Master.DashboardPassword = "pass"
	cfg.Recommend.ItemToItem = []config.ItemToItemConfig{
		{Name: "first", Type: "users", Column: "item.Labels"},
		{Name: "second", Type: "auto"},
	}
	// Tests seed recommendations directly; bulk imports must not trigger model training.
	cfg.Recommend.Collaborative.Type = "none"
	cfg.Recommend.UserToUser = []config.UserToUserConfig{
		{Name: "similar", Type: "embedding", Column: "user.Labels.embedding"},
	}
	cfg.Recommend.Collaborative.FitEpoch = 10
	cfg.Recommend.Collaborative.OptimizePeriod = time.Hour
	s.master = master.NewMaster(cfg, tempDir, true, "")
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		s.master.Serve()
	}()

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", cfg.Master.HttpPort)
	s.Require().Eventually(func() bool {
		resp, err := http.Get(endpoint + "/api/health/live")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		return true
	}, 10*time.Second, 10*time.Millisecond, "master did not become ready")

	t.Setenv("GORSE_ADMIN_ENDPOINT", endpoint)
	t.Setenv("GORSE_ADMIN_API_KEY", "test-key")
	s.root = &cobra.Command{Use: "gorse-cli", SilenceErrors: true, SilenceUsage: true}
	s.root.PersistentFlags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
	s.root.PersistentFlags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
	s.root.PersistentFlags().String("application", "", "Gorse CLI application name")
	s.root.AddCommand(ApplicationCmd, PipelineCmd, ItemCmd, UserCmd, FeedbackCmd, DumpCmd, RestoreCmd, GetStatsCmd, RecommendCmd)
}

func (s *CLITestSuite) TearDownTest() {
	s.NoError(s.master.DataClient.Purge())
	s.NoError(s.master.CacheClient.Purge())
	s.NoError(vectors.Purge(s.T().Context(), s.master.VectorClient))
}

func (s *CLITestSuite) TearDownSuite() {
	s.master.Shutdown()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		s.T().Error("master did not stop")
	}
}

func (s *CLITestSuite) EqualTable(expected [][]string, actual string) {
	s.T().Helper()
	var output bytes.Buffer
	table := tabwriter.NewWriter(&output, 0, 8, 2, ' ', 0)
	for _, row := range expected {
		_, err := fmt.Fprintln(table, strings.Join(row, "\t"))
		s.Require().NoError(err)
	}
	s.Require().NoError(table.Flush())
	lines := strings.Split(output.String(), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	s.Equal(strings.Join(lines, "\n"), actual)
}

func (s *CLITestSuite) Command(args ...string) (string, error) {
	s.T().Helper()
	commands := []*cobra.Command{s.root}
	for i := 0; i < len(commands); i++ {
		cmd := commands[i]
		commands = append(commands, cmd.Commands()...)
		for _, flags := range []*pflag.FlagSet{cmd.Flags(), cmd.PersistentFlags()} {
			flags.VisitAll(func(flag *pflag.Flag) {
				if value, ok := flag.Value.(pflag.SliceValue); ok {
					s.Require().NoError(value.Replace(nil))
				} else {
					s.Require().NoError(flag.Value.Set(flag.DefValue))
				}
				flag.Changed = false
			})
		}
	}
	s.root.SetArgs(args)
	var stdout, stderr bytes.Buffer
	s.root.SetOut(&stdout)
	s.root.SetErr(&stderr)
	err := s.root.Execute()
	s.Require().Empty(stderr.String())
	return stdout.String(), err
}

func (s *CLITestSuite) TestDumpRestore() {
	ctx := s.T().Context()
	timestamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	user := data.User{UserId: "backup-user", Labels: map[string]any{"group": "test"}, Comment: "backup user"}
	item := data.Item{ItemId: "backup-item", IsHidden: true, Categories: []string{"news", "tech"}, Timestamp: timestamp,
		Labels: map[string]any{"topic": "science"}, Comment: "backup item"}
	feedback := data.Feedback{FeedbackType: "click", UserId: user.UserId, ItemId: item.ItemId, Value: 2.5,
		Timestamp: timestamp, Labels: map[string]any{"source": "test"}, Comment: "backup feedback"}
	s.Require().NoError(s.master.DataClient.BatchInsertUsers(ctx, []data.User{user}))
	s.Require().NoError(s.master.DataClient.BatchInsertItems(ctx, []data.Item{item}))
	s.Require().NoError(s.master.DataClient.BatchInsertFeedback(ctx, []data.Feedback{feedback}, false, false, true))

	path := filepath.Join(s.T().TempDir(), "backup file.bin")
	out, err := s.Command("dump", path)
	s.Require().NoError(err)
	s.Equal("Data dumped to "+path+"\n", out)
	backup, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.NotEmpty(backup)

	s.Require().NoError(s.master.DataClient.Purge())
	count, err := s.master.DataClient.CountUsers(ctx)
	s.Require().NoError(err)
	s.Zero(count)
	count, err = s.master.DataClient.CountItems(ctx)
	s.Require().NoError(err)
	s.Zero(count)
	count, err = s.master.DataClient.CountFeedback(ctx)
	s.Require().NoError(err)
	s.Zero(count)

	s.root.SetIn(strings.NewReader(" Y "))
	defer s.root.SetIn(nil)
	out, err = s.Command("restore", path)
	s.Require().NoError(err)
	prefix := fmt.Sprintf("Restore data from %s? Existing users, items, feedback, and cache will be overwritten. Confirm [y/N]: Restored 1 users, 1 items, 1 feedback in ", path)
	s.Require().True(strings.HasPrefix(out, prefix), out)
	duration, err := time.ParseDuration(strings.TrimSuffix(strings.TrimPrefix(out, prefix), ".\n"))
	s.Require().NoError(err)
	s.GreaterOrEqual(duration, time.Duration(0))

	restoredUser, err := s.master.DataClient.GetUser(ctx, user.UserId)
	s.Require().NoError(err)
	restoredUser.UpdateAt = time.Time{}
	s.Equal(user, restoredUser)
	restoredItem, err := s.master.DataClient.GetItem(ctx, item.ItemId)
	s.Require().NoError(err)
	restoredItem.UpdateAt = time.Time{}
	s.Equal(item, restoredItem)
	restoredFeedback, err := s.master.DataClient.GetUserItemFeedback(ctx, user.UserId, item.ItemId)
	s.Require().NoError(err)
	s.Require().Len(restoredFeedback, 1)
	// Updated is generated by storage and is not part of the backup format.
	restoredFeedback[0].Updated = time.Time{}
	s.Equal(feedback, restoredFeedback[0])
}

func (s *CLITestSuite) TestRestoreCanceled() {
	ctx := s.T().Context()
	user := data.User{UserId: "keep-user", Comment: "not restored"}
	s.Require().NoError(s.master.DataClient.BatchInsertUsers(ctx, []data.User{user}))
	user, err := s.master.DataClient.GetUser(ctx, user.UserId)
	s.Require().NoError(err)
	path := filepath.Join(s.T().TempDir(), "missing.bin")
	defer s.root.SetIn(nil)
	for _, input := range []string{"n\n", "\n", "", "yes\n"} {
		s.root.SetIn(strings.NewReader(input))
		out, err := s.Command("restore", path)
		s.Require().NoError(err)
		s.Equal(fmt.Sprintf("Restore data from %s? Existing users, items, feedback, and cache will be overwritten. Confirm [y/N]: Restore canceled\n", path), out)
		restoredUser, err := s.master.DataClient.GetUser(ctx, user.UserId)
		s.Require().NoError(err)
		s.Equal(user, restoredUser)
	}
}

func (s *CLITestSuite) TestStats() {
	ctx := s.T().Context()
	s.Require().NoError(s.master.CacheClient.Set(ctx,
		cache.Integer(cache.Key(cache.GlobalMeta, cache.NumUsers), 7),
		cache.Integer(cache.Key(cache.GlobalMeta, cache.NumItems), 11),
		cache.Integer(cache.Key(cache.GlobalMeta, cache.NumUserLabels), 3),
		cache.Integer(cache.Key(cache.GlobalMeta, cache.NumItemLabels), 5),
		cache.Integer(cache.Key(cache.GlobalMeta, cache.NumTotalPosFeedbacks), 23),
		cache.Integer(cache.Key(cache.GlobalMeta, cache.NumValidPosFeedbacks), 17),
		cache.Integer(cache.Key(cache.GlobalMeta, cache.NumValidNegFeedbacks), 2),
		cache.Time(cache.Key(cache.GlobalMeta, cache.LastUpdatePopularItemsTime), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
	))
	out, err := s.Command("stats")
	s.Require().NoError(err)
	for _, line := range []string{
		"BinaryVersion:          ",
		"MatchingModelScore:\n{\n\t",
		"NumItemLabels:          5\n",
		"NumItems:               11\n",
		"NumServers:             0\n",
		"NumTotalPosFeedback:    23\n",
		"NumUserLabels:          3\n",
		"NumUsers:               7\n",
		"NumValidNegFeedback:    2\n",
		"NumValidPosFeedback:    17\n",
		"NumWorkers:             0\n",
		"PopularItemsUpdateTime: 2026-01-02T03:04:05Z\n",
		"RankingModelScore:\n{\n\t",
	} {
		s.Contains(out, line)
	}
}
