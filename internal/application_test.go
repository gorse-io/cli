package internal

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestApplications(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GORSE_ADMIN_ENDPOINT", "")
	t.Setenv("GORSE_ADMIN_API_KEY", "")
	root := &cobra.Command{Use: "gorse-cli"}
	root.PersistentFlags().String("endpoint", "", "Gorse base URL (default: selected application or GORSE_ADMIN_ENDPOINT)")
	root.PersistentFlags().String("api-key", "", "Gorse admin API key (default: selected application or GORSE_ADMIN_API_KEY)")
	root.PersistentFlags().String("application", "", "Gorse CLI application name")
	root.AddCommand(ApplicationCmd)
	t.Cleanup(func() { root.RemoveCommand(ApplicationCmd) })

	// An empty keyring has no configured or current application.
	stdout, stderr := Command(t, root, "application", "list")
	require.Empty(t, stderr)
	require.Equal(t, "No applications configured.\n", stdout)
	stdout, stderr = Command(t, root, "application", "current")
	require.Empty(t, stderr)
	require.Equal(t, "No current application.\n", stdout)

	// Adding an application saves its endpoint and selects it without exposing the API key.
	stdout, stderr = Command(t, root, "application", "add", "dev", "--endpoint", "http://localhost:8088", "--api-key", "test-key")
	require.Empty(t, stderr)
	require.Equal(t, "Application \"dev\" saved and selected.\n", stdout)
	stdout, stderr = Command(t, root, "application", "current")
	require.Empty(t, stderr)
	require.Equal(t, "Name:\t\tdev\nEndpoint:\thttp://localhost:8088\n", stdout)

	// Adding a second application makes it the current application.
	stdout, stderr = Command(t, root, "application", "add", "prod", "--endpoint", "http://localhost:8089", "--api-key", "prod-key")
	require.Empty(t, stderr)
	require.Equal(t, "Application \"prod\" saved and selected.\n", stdout)
	stdout, stderr = Command(t, root, "application", "current")
	require.Empty(t, stderr)
	require.Equal(t, "Name:\t\tprod\nEndpoint:\thttp://localhost:8089\n", stdout)

	// Adding an existing name updates and selects it without creating a duplicate.
	stdout, stderr = Command(t, root, "application", "add", "dev", "--endpoint", "http://localhost:8090", "--api-key", "updated-key")
	require.Empty(t, stderr)
	require.Equal(t, "Application \"dev\" saved and selected.\n", stdout)
	stdout, stderr = Command(t, root, "application", "current")
	require.Empty(t, stderr)
	require.Equal(t, "Name:\t\tdev\nEndpoint:\thttp://localhost:8090\n", stdout)
	// The list is sorted, marks the current application, and omits API keys.
	stdout, stderr = Command(t, root, "application", "list")
	require.Empty(t, stderr)
	require.Equal(t, []string{
		"CURRENT", "NAME", "ENDPOINT",
		"*", "dev", "http://localhost:8090",
		"prod", "http://localhost:8089",
	}, strings.Fields(stdout))

	// Switching applications changes the current selection while retaining saved endpoints.
	stdout, stderr = Command(t, root, "application", "use", "prod")
	require.Empty(t, stderr)
	require.Equal(t, "Switched to application \"prod\".\n", stdout)
	stdout, stderr = Command(t, root, "application", "current")
	require.Empty(t, stderr)
	require.Equal(t, "Name:\t\tprod\nEndpoint:\thttp://localhost:8089\n", stdout)
	stdout, stderr = Command(t, root, "application", "use", "dev")
	require.Empty(t, stderr)
	require.Equal(t, "Switched to application \"dev\".\n", stdout)

	// Deleting the current application selects the remaining application.
	stdout, stderr = Command(t, root, "application", "delete", "dev")
	require.Empty(t, stderr)
	require.Equal(t, "Application \"dev\" deleted.\n", stdout)
	stdout, stderr = Command(t, root, "application", "current")
	require.Empty(t, stderr)
	require.Equal(t, "Name:\t\tprod\nEndpoint:\thttp://localhost:8089\n", stdout)
	stdout, stderr = Command(t, root, "application", "list")
	require.Empty(t, stderr)
	require.Equal(t, []string{"CURRENT", "NAME", "ENDPOINT", "*", "prod", "http://localhost:8089"}, strings.Fields(stdout))

	// Deleting the last application clears both the list and the current selection.
	stdout, stderr = Command(t, root, "application", "delete", "prod")
	require.Empty(t, stderr)
	require.Equal(t, "Application \"prod\" deleted.\n", stdout)
	stdout, stderr = Command(t, root, "application", "list")
	require.Empty(t, stderr)
	require.Equal(t, "No applications configured.\n", stdout)
	stdout, stderr = Command(t, root, "application", "current")
	require.Empty(t, stderr)
	require.Equal(t, "No current application.\n", stdout)
}
