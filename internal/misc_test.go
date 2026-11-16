package internal

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorse-io/gorse/common/log"
	"github.com/gorse-io/gorse/config"
	"github.com/gorse-io/gorse/master"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/suite"
)

type CLITestSuite struct {
	suite.Suite
	root   *cobra.Command
	master *master.Master
	done   chan struct{}
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
	cfg.Recommend.Collaborative.Type = "mf"
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
	s.root.AddCommand(ApplicationCmd, PipelineCmd)
}

func (s *CLITestSuite) TearDownSuite() {
	s.master.Shutdown()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		s.T().Error("master did not stop")
	}
}

func (s *CLITestSuite) Command(args ...string) string {
	s.T().Helper()
	s.root.SetArgs(args)
	var stdout, stderr bytes.Buffer
	s.root.SetOut(&stdout)
	s.root.SetErr(&stderr)
	s.Require().NoError(s.root.Execute())
	s.Require().Empty(stderr.String())
	return stdout.String()
}
