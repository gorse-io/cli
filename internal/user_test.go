package internal

import (
	"time"

	"github.com/gorse-io/gorse/storage"
)

func (s *CLITestSuite) TestUserUpdate() {
	ctx := s.T().Context()
	_, err := s.Command("user", "add", "updated-user", "--comment", "original comment", "--labels", `{"role":"reader","keep":"original"}`)
	s.Require().NoError(err)
	expected, err := s.master.DataClient.GetUser(ctx, "updated-user")
	s.Require().NoError(err)
	expected.UpdateAt = time.Time{}
	check := func() {
		s.T().Helper()
		actual, err := s.master.DataClient.GetUser(ctx, "updated-user")
		s.Require().NoError(err)
		actual.UpdateAt = time.Time{}
		s.Equal(expected, actual)
	}

	out, err := s.Command("user", "update", "updated-user", "--comment", "updated comment")
	s.Require().NoError(err)
	s.Contains(out, "RowAffected: 1")
	expected.Comment = "updated comment"
	check()
	_, err = s.Command("user", "update", "updated-user", "--labels", `{"role":"writer"}`)
	s.Require().NoError(err)
	expected.Labels = map[string]any{"role": "writer"}
	check()
	_, err = s.Command("user", "update", "updated-user", "--comment=")
	s.Require().NoError(err)
	expected.Comment = ""
	check()
	_, err = s.Command("user", "update", "updated-user", "--labels", "{}")
	s.Require().NoError(err)
	expected.Labels = map[string]any{}
	check()
	_, err = s.Command("user", "update", "updated-user", "--comment", "final comment", "--labels", "[]")
	s.Require().NoError(err)
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
		{[]string{"--labels", "true", "--comment", "must not change"}, "API request failed"},
	} {
		before, err := s.master.DataClient.GetUser(ctx, "updated-user")
		s.Require().NoError(err)
		out, err = s.Command(append([]string{"user", "update", "updated-user"}, tc.flags...)...)
		s.ErrorContains(err, tc.error)
		s.Empty(out)
		after, err := s.master.DataClient.GetUser(ctx, "updated-user")
		s.Require().NoError(err)
		s.Equal(before, after)
	}
	_, err = s.Command("user", "update")
	s.ErrorContains(err, "accepts 1 arg(s)")
}

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
