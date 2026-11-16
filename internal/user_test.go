package internal

import (
	"github.com/gorse-io/gorse/storage"
	"github.com/spf13/pflag"
)

func (s *CLITestSuite) TestUserAddDelete() {
	defer func() {
		for _, cmd := range []*pflag.FlagSet{addUserCmd.Flags(), deleteUserCmd.Flags()} {
			cmd.VisitAll(func(flag *pflag.Flag) {
				s.NoError(flag.Value.Set(flag.DefValue))
				flag.Changed = false
			})
		}
	}()

	s.Contains(s.Command("user", "add", "minimal-user"), "RowAffected: 1")
	user, err := s.master.DataClient.GetUser(s.T().Context(), "minimal-user")
	s.Require().NoError(err)
	s.Empty(user.Comment)

	out := s.Command("user", "add", "added-user", "--comment", "new user", "--labels", `{"role":"reader"}`)
	s.Contains(out, "RowAffected: 1")
	user, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
	s.Require().NoError(err)
	s.Equal("new user", user.Comment)
	s.Equal("reader", user.Labels.(map[string]any)["role"])

	for _, flags := range [][]string{nil, {"--yes=false"}} {
		s.root.SetArgs(append([]string{"user", "delete", "added-user"}, flags...))
		s.Require().ErrorContains(s.root.Execute(), "--yes")
		_, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
		s.Require().NoError(err)
	}
	s.Contains(s.Command("user", "delete", "added-user", "--yes"), "RowAffected: 1")
	_, err = s.master.DataClient.GetUser(s.T().Context(), "added-user")
	s.ErrorIs(err, storage.ErrNotFound)
}
