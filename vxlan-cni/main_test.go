package main

import (
	"testing"

	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/stretchr/testify/require"
)

func TestCmdAdd(t *testing.T) {
	originalNS, err := testutils.NewNS()
	require.NoError(t, err)
	defer func() {
		_ = originalNS.Close()
		_ = testutils.UnmountNS(originalNS)
	}()

	targetNS, err := testutils.NewNS()
	require.NoError(t, err)
	defer func() {
		_ = targetNS.Close()
		_ = testutils.UnmountNS(targetNS)
	}()

	// TODO: ...
}
