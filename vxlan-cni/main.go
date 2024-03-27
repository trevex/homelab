package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
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

func ensureBridge(n *NetConf, brName string) (*netlink.Bridge, error) {
	err := netlink.LinkAdd(&netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   brName,
			MTU:    n.MTU,
			TxQLen: -1,
		},
	})

	// We ignore error if it the bridge already existed
	if err != nil && err != syscall.EEXIST {
		return nil, fmt.Errorf("could not add %q: %v", brName, err)
	}

	// Fetch the bridge
	br, err := bridgeByName(brName)
	if err != nil {
		return nil, err
	}

	// We want to own the routes for this interface
	_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv6/conf/%s/accept_ra", brName), "0")

	// We disable Spanning Tree Protocol
	if err := os.WriteFile(fmt.Sprintf("/sys/class/net/%s/bridge/stp_state", brName), []byte("0"), 0644); err != nil {
		return nil, err
	}

	// And make sure it is up
	if err := netlink.LinkSetUp(br); err != nil {
		return nil, err
	}

	return br, nil
}

func ensureVXLAN(n *NetConf, vxName string, br *netlink.Bridge) (netlink.Link, error) {
	nodeIP, err := getDefaultLinkIP()
	if err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("could not add %q: %v", vxName, err)
	}

	// Fetch the link
	vx, err := netlink.LinkByName(vxName)
	if err := netlink.LinkSetMaster(vx, br); err != nil {
		return nil, fmt.Errorf("failed to connect %q to bridge %v: %v", vx.Attrs().Name, br.Attrs().Name, err)
	}

	// And make sure it is up
	if err := netlink.LinkSetUp(vx); err != nil {
		return nil, err
	}

	return vx, nil
}

func createVeth(n *NetConf, ifName string, netns ns.NetNS, br *netlink.Bridge) (string, string, error) {
	hostIfName := ""
	contIfName := ""

	err := netns.Do(func(hostNS ns.NetNS) error {
		hostVeth, contVeth, err := ip.SetupVeth(ifName, n.MTU, "", hostNS)
		hostIfName = hostVeth.Name
		contIfName = contVeth.Name
		return err
	})
	if err != nil {
		return hostIfName, contIfName, err
	}

	// Need to lookup hostVeth again as its index has changed during ns move
	hostVeth, err := netlink.LinkByName(hostIfName)
	if err != nil {
		return hostIfName, contIfName, fmt.Errorf("failed to lookup %q: %v", ifName, err)
	}

	// connect host veth end to the bridge
	if err := netlink.LinkSetMaster(hostVeth, br); err != nil {
		return hostIfName, contIfName, fmt.Errorf("failed to connect %q to bridge %v: %v", hostVeth.Attrs().Name, br.Attrs().Name, err)
	}

	return hostIfName, contIfName, nil
}

func resultForInterfaces(netns ns.NetNS, br *netlink.Bridge, hostIfName, contIfName string) (*current.Result, error) {
	hostIf := &current.Interface{
		Name: hostIfName,
	}
	hostVeth, err := netlink.LinkByName(hostIf.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup %q: %v", hostIf.Name, err)
	}
	hostIf.Mac = hostVeth.Attrs().HardwareAddr.String()

	contIf := &current.Interface{
		Name: contIfName,
	}
	err = netns.Do(func(hostNS ns.NetNS) error {
		contVeth, err := netlink.LinkByName(contIfName)
		if err != nil {
			return fmt.Errorf("failed to lookup %q: %v", contIfName, err)
		}
		contIf.Mac = contVeth.Attrs().HardwareAddr.String()
		contIf.Sandbox = netns.Path()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &current.Result{
		CNIVersion: current.ImplementedSpecVersion,
		Interfaces: []*current.Interface{
			{
				Name: br.Attrs().Name,
				Mac:  br.Attrs().HardwareAddr.String(),
			},
			hostIf,
			contIf,
		},
	}, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	success := false

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
	br, err := ensureBridge(n, brName)
	if err != nil {
		return err
	}

	// Let's make sure the vxlan interface exists (otherwise we create it)
	vxName := fmt.Sprintf("vxlan%d", n.VNI)
	_, err = ensureVXLAN(n, vxName, br)

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	// Let's create the container and host interface
	ifName := args.IfName
	hostIfName, contIfName, err := createVeth(n, ifName, netns, br)

	// Start preparing the result with our interfaces
	result, err := resultForInterfaces(netns, br, hostIfName, contIfName)
	if err != nil {
		return err
	}

	// Let's retrieve the IP and Prefix from IPAM
	if n.IPAM.Type != "" {
		r, err := ipam.ExecAdd(n.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}

		// Release IP in case of failure
		defer func() {
			if !success {
				ipam.ExecDel(n.IPAM.Type, args.StdinData)
			}
		}()

		// Convert whatever the IPAM result was into the current Result type
		ipamResult, err := current.NewResultFromResult(r)
		if err != nil {
			return err
		}

		result.IPs = ipamResult.IPs
		result.Routes = ipamResult.Routes
		result.DNS = ipamResult.DNS

		if len(result.IPs) == 0 {
			return errors.New("IPAM plugin returned missing IP config")
		}

		// Configure the container hardware address and IP address(es)
		if err := netns.Do(func(_ ns.NetNS) error {
			_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv6/conf/%s/accept_dad", contIfName), "0")
			_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/arp_notify", contIfName), "1")
			return ipam.ConfigureIface(contIfName, result)
		}); err != nil {
			return err
		}
	} else { // ipam.Configure will set link to up, if ipam not used, we have to manually do it
		if err := netns.Do(func(_ ns.NetNS) error {
			link, err := netlink.LinkByName(contIfName)
			if err != nil {
				return fmt.Errorf("failed to retrieve link: %v", err)
			}

			if err = netlink.LinkSetUp(link); err != nil {
				return fmt.Errorf("failed to set %q up: %v", contIfName, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}

	success = true

	return types.PrintResult(result, n.CNIVersion)
}

func bridgeIfCount(brName string) (int, error) {
	ifs, err := os.ReadDir(fmt.Sprintf("/sys/class/net/%s/brif", brName))

	if err != nil {
		return -1, err
	}
	return len(ifs), nil
}

func cmdDel(args *skel.CmdArgs) error {
	n, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if args.Netns != "" {
		err = ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
			_, err := ip.DelLinkByNameAddr(args.IfName)
			if err != nil && err == ip.ErrLinkNotFound {
				return nil
			}
			return err
		})
		if err != nil {
			// https://github.com/kubernetes/kubernetes/issues/43014#issuecomment-287164444
			_, ok := err.(ns.NSPathNotExistErr)
			if !ok { // ignore NSPathNotExistErr
				return err
			}
		}
	}

	// We want to make sure only a single instance tampers with a particular
	// VXLAN bridge and interface to avoid race-conditions.
	l, err := LockVNI(n.VNI)
	if err != nil {
		return err
	}
	defer l.Unlock()

	vxName := fmt.Sprintf("vxlan%d", n.VNI)
	brName := fmt.Sprintf("bridge%d", n.VNI)
	brIfCount, err := bridgeIfCount(brName)
	if err != nil {
		return err
	}
	if brIfCount <= 1 {
		vx, _ := netlink.LinkByName(vxName)
		if vx != nil { // If not already deletead
			if err := netlink.LinkDel(vx); err != nil {
				return err
			}
		}

		br, _ := netlink.LinkByName(brName)
		if br != nil { // If not already deletead
			if err := netlink.LinkDel(br); err != nil {
				return err
			}
		}
	}

	return ipam.ExecDel(n.IPAM.Type, args.StdinData)
}

func cmdCheck(args *skel.CmdArgs) error {
	// TODO: implement
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("vxlan"))
}
