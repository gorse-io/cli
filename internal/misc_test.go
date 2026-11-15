package internal

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Command(t *testing.T, root *cobra.Command, args ...string) (string, string) {
	t.Helper()
	root.SetArgs(args)
	buf1 := new(bytes.Buffer)
	buf2 := new(bytes.Buffer)
	root.SetOut(buf1)
	root.SetErr(buf2)
	require.NoError(t, root.Execute())
	return buf1.String(), buf2.String()
}
