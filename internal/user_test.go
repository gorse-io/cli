package internal

import (
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

	expected := [][]string{
		{"USER-ID", "COMMENT", "LABELS"},
		{"added-user", "new user", `{"role":"reader"}`},
		{"minimal-user", "", "{}"},
	}
	out, err = s.Command("user", "list", "-n", "1")
	s.Require().NoError(err)
	s.EqualTable(expected[:2], out)

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
	s.Contains(out, "RowAffected: 1")
	_, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
	s.ErrorIs(err, storage.ErrNotFound)
}
