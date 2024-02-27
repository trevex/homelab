package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"golang.org/x/sys/unix"
)

type Lockfile int

func Lock(path string) (Lockfile, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC, 0600)
	if err != nil {
		return Lockfile(-1), err
	}

	// TODO: Implement TryLock taking context and using LOCK_NB instead!?
	return Lockfile(fd), unix.Flock(fd, unix.LOCK_EX)
}

func (fd Lockfile) Unlock() error {
	return unix.Flock(int(fd), unix.LOCK_UN)
}

func LockVNI(vni int) (Lockfile, error) {
	return Lock(os.TempDir() + fmt.Sprintf("/vxlan%d.lock", vni))
}

type NetConf struct {
	types.NetConf
	VNI int `json:"vni"`
}

func loadNetConf(bytes []byte) (*NetConf, error) {
	n := &NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	if n.VNI < 1 || n.VNI > 16000000 { // TODO: double check maximum number...
		return nil, fmt.Errorf("invalid VNI %d (must be between 1 and 16M)", n.VNI)
	}
	return n, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	n, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	// We want to make sure only a single instance tampers with a particular
	// VXLAN bridge and interface to avoid race-conditions.
	l, err := LockVNI(n.VNI)
	if err != nil {
		return err
	}
	defer l.Unlock()

	// Let's make sure the bridge exists (otherwise we create it)
	// TODO

	// Let's make sure the vxlan interface exists (otherwise we create it)
	// TODO

	// Let's retrieve the IP and Prefix from IPAM
	// TODO

	// Let's create the container and host interface
	// TODO

	// Next we assign the IP to the container interface

	// Up!

	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("vxlan"))
}
