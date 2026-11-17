package internal

import (
	"strings"

	"github.com/gorse-io/gorse/storage"
)

func (s *CLITestSuite) TestUser() {
	out, err := s.Command("user", "add", "added-user", "--comment", "new user", "--labels", `{"role":"reader"}`)
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1")
	user, err := s.master.DataClient.GetUser(s.T().Context(), "added-user")
	s.Require().NoError(err)
	s.Equal("new user", user.Comment)
	s.Equal("reader", user.Labels.(map[string]any)["role"])

	out, err = s.Command("user", "add", "minimal-user")
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1")
	user, err = s.master.DataClient.GetUser(s.T().Context(), "minimal-user")
	s.Require().NoError(err)
	s.Empty(user.Comment)

	out, err = s.Command("user", "get", "added-user")
	s.Require().NoError(err)
	s.Equal(`UserId: added-user
Comment: new user
Labels:
{
  "role": "reader"
}
`, out)

	out, err = s.Command("user", "list", "-n", "1")
	s.Require().NoError(err)
	s.Len(strings.Split(strings.TrimSpace(out), "\n"), 2)
	s.Regexp(`(?m)^USER-ID\s+COMMENT\s+LABELS$`, out)

	out, err = s.Command("user", "list")
	s.Require().NoError(err)
	s.Len(strings.Split(strings.TrimSpace(out), "\n"), 3)
	s.Regexp(`(?m)^added-user\s+new user\s+\{"role":"reader"\}$`, out)
	s.Regexp(`(?m)^minimal-user\s+\{\}$`, out)

	for _, flags := range [][]string{nil, {"--yes=false"}} {
		_, err = s.Command(append([]string{"user", "delete", "added-user"}, flags...)...)
		s.Require().ErrorContains(err, "--yes")
		_, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
		s.Require().NoError(err)
	}
	out, err = s.Command("user", "delete", "added-user", "--yes")
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1")
	_, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
	s.ErrorIs(err, storage.ErrNotFound)
}
