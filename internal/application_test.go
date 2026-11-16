package internal

import (
	"strings"

	"github.com/zalando/go-keyring"
)

func (s *CLITestSuite) TestApplications() {
	keyring.MockInit()

	// An empty keyring has no configured or current application.
	stdout := s.Command("application", "list")
	s.Equal("No applications configured.\n", stdout)
	stdout = s.Command("application", "current")
	s.Equal("No current application.\n", stdout)

	// Adding an application saves its endpoint and selects it without exposing the API key.
	stdout = s.Command("application", "add", "dev", "--endpoint", "http://localhost:8088", "--api-key", "test-key")
	s.Equal("Application \"dev\" saved and selected.\n", stdout)
	stdout = s.Command("application", "current")
	s.Equal("Name:\t\tdev\nEndpoint:\thttp://localhost:8088\n", stdout)

	// Adding a second application makes it the current application.
	stdout = s.Command("application", "add", "prod", "--endpoint", "http://localhost:8089", "--api-key", "prod-key")
	s.Equal("Application \"prod\" saved and selected.\n", stdout)
	stdout = s.Command("application", "current")
	s.Equal("Name:\t\tprod\nEndpoint:\thttp://localhost:8089\n", stdout)

	// Adding an existing name updates and selects it without creating a duplicate.
	stdout = s.Command("application", "add", "dev", "--endpoint", "http://localhost:8090", "--api-key", "updated-key")
	s.Equal("Application \"dev\" saved and selected.\n", stdout)
	stdout = s.Command("application", "current")
	s.Equal("Name:\t\tdev\nEndpoint:\thttp://localhost:8090\n", stdout)
	// The list is sorted, marks the current application, and omits API keys.
	stdout = s.Command("application", "list")
	s.Equal([]string{
		"CURRENT", "NAME", "ENDPOINT",
		"*", "dev", "http://localhost:8090",
		"prod", "http://localhost:8089",
	}, strings.Fields(stdout))

	// Switching applications changes the current selection while retaining saved endpoints.
	stdout = s.Command("application", "use", "prod")
	s.Equal("Switched to application \"prod\".\n", stdout)
	stdout = s.Command("application", "current")
	s.Equal("Name:\t\tprod\nEndpoint:\thttp://localhost:8089\n", stdout)
	stdout = s.Command("application", "use", "dev")
	s.Equal("Switched to application \"dev\".\n", stdout)

	// Deleting the current application selects the remaining application.
	stdout = s.Command("application", "delete", "dev")
	s.Equal("Application \"dev\" deleted.\n", stdout)
	stdout = s.Command("application", "current")
	s.Equal("Name:\t\tprod\nEndpoint:\thttp://localhost:8089\n", stdout)
	stdout = s.Command("application", "list")
	s.Equal([]string{"CURRENT", "NAME", "ENDPOINT", "*", "prod", "http://localhost:8089"}, strings.Fields(stdout))

	// Deleting the last application clears both the list and the current selection.
	stdout = s.Command("application", "delete", "prod")
	s.Equal("Application \"prod\" deleted.\n", stdout)
	stdout = s.Command("application", "list")
	s.Equal("No applications configured.\n", stdout)
	stdout = s.Command("application", "current")
	s.Equal("No current application.\n", stdout)
}
