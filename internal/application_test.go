package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestApplications(t *testing.T) {
	keyring.MockInit()
	applications := NewApplications("test-applications")

	names, err := applications.List()
	require.NoError(t, err)
	require.Empty(t, names)
	current, err := applications.Current()
	require.NoError(t, err)
	require.Empty(t, current)
	require.NoError(t, applications.ClearCurrent())

	require.NoError(t, applications.Upsert("dev", "http://localhost:8088", "test-key"))
	app, err := applications.Get("dev")
	require.NoError(t, err)
	require.Equal(t, Application{Name: "dev", Endpoint: "http://localhost:8088", APIKey: "test-key"}, app)
	current, err = applications.Current()
	require.NoError(t, err)
	require.Equal(t, "dev", current)

	require.NoError(t, applications.Upsert("prod", "http://localhost:8089", "prod-key"))
	require.NoError(t, applications.Upsert("dev", "http://localhost:8090", "updated-key"))
	app, err = applications.Get("dev")
	require.NoError(t, err)
	require.Equal(t, Application{Name: "dev", Endpoint: "http://localhost:8090", APIKey: "updated-key"}, app)
	names, err = applications.List()
	require.NoError(t, err)
	require.Equal(t, []string{"dev", "prod"}, names)

	require.NoError(t, applications.SetCurrent("prod"))
	current, err = applications.Current()
	require.NoError(t, err)
	require.Equal(t, "prod", current)
	require.NoError(t, applications.ClearCurrent())
	current, err = applications.Current()
	require.NoError(t, err)
	require.Empty(t, current)
	require.NoError(t, applications.SetCurrent("dev"))

	require.NoError(t, applications.Delete("dev"))
	_, err = applications.Get("dev")
	require.ErrorContains(t, err, `application "dev" not found`)
	names, err = applications.List()
	require.NoError(t, err)
	require.Equal(t, []string{"prod"}, names)
	current, err = applications.Current()
	require.NoError(t, err)
	require.Equal(t, "prod", current)
	require.NoError(t, applications.Delete("prod"))
	names, err = applications.List()
	require.NoError(t, err)
	require.Empty(t, names)
	current, err = applications.Current()
	require.NoError(t, err)
	require.Empty(t, current)
	require.ErrorContains(t, applications.Delete("prod"), `application "prod" not found`)
}
