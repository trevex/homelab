package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/cybozu-go/netutil"
	"github.com/vishvananda/netlink"
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
	VNI  int `json:"vni"`
	MTU  int `json:"mtu"`
	Port int `json:"port"`
}

func loadNetConf(bytes []byte) (*NetConf, error) {
	n := &NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	if n.VNI < 1 || n.VNI > 16000000 { // TODO: double check maximum number...
		return nil, fmt.Errorf("invalid VNI %d (must be between 1 and 16M)", n.VNI)
	}

	if n.MTU == 0 {
		n.MTU, _ = netutil.DetectMTU()
	}
	if n.MTU < 1280 || n.MTU > 9216 { // TODO: should Jumbo frames be included?
		return nil, fmt.Errorf("invalid MTU: %d", n.MTU)
	}

	if n.Port == 0 {
		n.Port = 4789
	}
	if n.Port < 1024 || n.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", n.Port)
	}

	return n, nil
}

func bridgeByName(name string) (*netlink.Bridge, error) {
	l, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("could not lookup %q: %v", name, err)
	}
	br, ok := l.(*netlink.Bridge)
	if !ok {
		return nil, fmt.Errorf("%q already exists but is not a bridge", name)
	}
	return br, nil
}

func getDefaultRouteIfName() (int, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return -1, err
	}

	for _, route := range routes {
		if route.Dst == nil {
			return route.LinkIndex, nil
		}
	}

	return -1, fmt.Errorf("can not find default route interface")
}

func getDefaultLinkIP() (net.IP, error) {
	index, err := getDefaultRouteIfName()
	if err != nil {
		return nil, err
	}

	link, err := netlink.LinkByIndex(index)
	if err != nil {
		return nil, err
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	}

	return addrs[0].IP, nil // TODO: safe?
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

	// Let's create the bridge
	brName := fmt.Sprintf("bridge%d", n.VNI)
	err = netlink.LinkAdd(&netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   brName,
			MTU:    n.MTU,
			TxQLen: -1,
		},
	})
	// We ignore error if it the bridge already existed
	if err != nil && err != syscall.EEXIST {
		return fmt.Errorf("could not add %q: %v", brName, err)
	}
	// Fetch the bridge
	br, err := bridgeByName(brName)
	if err != nil {
		return err
	}
	// We want to own the routes for this interface
	_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv6/conf/%s/accept_ra", brName), "0")
	// We disable Spanning Tree Protocol
	if err := os.WriteFile(fmt.Sprintf("/sys/class/net/%s/bridge/stp_state", brName), []byte("0"), 0644); err != nil {
		return err
	}
	// And make sure it is up
	if err := netlink.LinkSetUp(br); err != nil {
		return err
	}

	// For the VXLAN interface we'll need the node IP
	nodeIP, err := getDefaultLinkIP()
	if err != nil {
		return err
	}

	// Let's make sure the vxlan interface exists (otherwise we create it)
	vxName := fmt.Sprintf("vxlan%d", n.VNI)
	err = netlink.LinkAdd(&netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: vxName,
		},
		VxlanId:  n.VNI,
		Port:     n.Port,
		Learning: false,
		SrcAddr:  nodeIP,
	})
	// We ignore error if it the bridge already existed
	if err != nil && err != syscall.EEXIST {
		return fmt.Errorf("could not add %q: %v", vxName, err)
	}
	// Fetch the link
	vx, err := netlink.LinkByName(vxName)
	if err := netlink.LinkSetMaster(vx, br); err != nil {
		return fmt.Errorf("failed to connect %q to bridge %v: %v", vx.Attrs().Name, br.Attrs().Name, err)
	}
	// And make sure it is up
	if err := netlink.LinkSetUp(vx); err != nil {
		return err
	}

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
