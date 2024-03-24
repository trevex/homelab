package main

import (
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ns"
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

	ifname := "testeth0"
	conf := `{
	"cniVersion": "0.3.0",
	"name": "cni-plugin-sample-test",
	"type": "vxlan",
	"vni": 321,
	"mtu": 1400
}`

	args := &skel.CmdArgs{
		ContainerID: "dummy",
		Netns:       targetNS.Path(),
		IfName:      ifname,
		StdinData:   []byte(conf),
	}

	err = originalNS.Do(func(ns.NetNS) error {
		_, _, err := testutils.CmdAddWithArgs(args, func() error { return cmdAdd(args) })
		if err != nil {
			return err
		}
		return testutils.CmdDelWithArgs(args, func() error { return cmdDel(args) })
	})
	require.NoError(t, err)
}
