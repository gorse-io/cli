package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestApplicationCommands(t *testing.T) {
	keyring.MockInit()
	t.Setenv("GORSE_ADMIN_ENDPOINT", "")
	t.Setenv("GORSE_ADMIN_API_KEY", "")

	out, err := executeRawCommand(rootCmd, "", "application", "add", "dev", "--endpoint", "http://localhost:8088", "--api-key", "test-key")
	require.NoError(t, err)
	require.Contains(t, out, `Application "dev" saved and selected.`)
	out, err = executeRawCommand(rootCmd, "", "application", "current")
	require.NoError(t, err)
	require.Contains(t, out, "dev")
	require.Contains(t, out, "http://localhost:8088")
	require.NotContains(t, out, "test-key")
	out, err = executeRawCommand(rootCmd, "", "application", "list")
	require.NoError(t, err)
	require.Contains(t, out, "dev")
	require.NotContains(t, out, "test-key")

	_, err = executeRawCommand(rootCmd, "", "application", "add", "prod", "--endpoint", "http://localhost:8089", "--api-key", "prod-key")
	require.NoError(t, err)
	out, err = executeRawCommand(rootCmd, "", "application", "use", "dev")
	require.NoError(t, err)
	require.Contains(t, out, `Switched to application "dev".`)

	resetFlags(rootCmd)
	require.NoError(t, getItemCmd.Flags().Set("application", "prod"))
	endpoint, apiKey := getEndpointAndKey(getItemCmd)
	require.Equal(t, "http://localhost:8089", endpoint)
	require.Equal(t, "prod-key", apiKey)
	resetFlags(rootCmd)
	endpoint, apiKey = getEndpointAndKey(getItemCmd)
	require.Equal(t, "http://localhost:8088", endpoint)
	require.Equal(t, "test-key", apiKey)

	out, err = executeRawCommand(rootCmd, "", "application", "delete", "dev")
	require.NoError(t, err)
	require.Contains(t, out, `Application "dev" deleted.`)
	current, err := applications.CurrentName()
	require.NoError(t, err)
	require.Equal(t, "prod", current)
	_, err = executeRawCommand(rootCmd, "", "application", "delete", "prod")
	require.NoError(t, err)
	out, err = executeRawCommand(rootCmd, "", "application", "current")
	require.NoError(t, err)
	require.Contains(t, out, "No current application.")

	_, err = executeRawCommand(rootCmd, "", "context", "list")
	require.Error(t, err)
	_, err = executeRawCommand(rootCmd, "", "item", "get", "item-1", "--context", "dev")
	require.Error(t, err)
}

func TestApplicationLoadsSavedCredentials(t *testing.T) {
	keyring.MockInit()
	// Saved credentials predate the user-facing terminology change.
	require.NoError(t, keyring.Set("gorse-cli", "contexts", `["dev"]`))
	require.NoError(t, keyring.Set("gorse-cli", "current-context", "dev"))
	require.NoError(t, keyring.Set("gorse-cli", "context:dev:admin-endpoint", "http://localhost:8088"))
	require.NoError(t, keyring.Set("gorse-cli", "context:dev:admin-api-key", "saved-key"))
	app, err := applications.Load("dev")
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8088", app.Endpoint)
	require.Equal(t, "saved-key", app.APIKey)
	names, err := applications.Names()
	require.NoError(t, err)
	require.Equal(t, []string{"dev"}, names)
	current, err := applications.CurrentName()
	require.NoError(t, err)
	require.Equal(t, "dev", current)
}
