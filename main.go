//go:build linux
// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/nohns/xvm-cni/pkg/ipam"
	"github.com/nohns/xvm-cni/pkg/vxlan"
)

// PluginConf represents the plugin configuration
type PluginConf struct {
	types.NetConf

	// Plugin-specific fields
	HostInterface string `json:"hostInterface"`
	VxlanID       int    `json:"vxlanID"`
	MTU           int    `json:"mtu"`
	Subnet        string `json:"subnet"`
	Gateway       string `json:"gateway"`
	DataDir       string `json:"dataDir"`
}

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("xvm-cni"))
}

func cmdAdd(args *skel.CmdArgs) error {
	// Parse network configuration
	conf := &PluginConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse network configuration: %v", err)
	}

	// Set default values if not specified
	if conf.VxlanID == 0 {
		conf.VxlanID = vxlan.DefaultVxlanVNI
	}
	if conf.MTU == 0 {
		conf.MTU = vxlan.DefaultMTU
	}
	if conf.HostInterface == "" {
		return fmt.Errorf("hostInterface must be specified")
	}
	if conf.Subnet == "" {
		return fmt.Errorf("subnet must be specified")
	}
	if conf.Gateway == "" {
		return fmt.Errorf("gateway must be specified")
	}

	// Enable IP forwarding
	_, err := sysctl.Sysctl("net.ipv4.ip_forward", "1")
	if err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %v", err)
	}

	// Setup VXLAN network
	vxlanConfig := &vxlan.VxlanConfig{
		HostInterface: conf.HostInterface,
		VxlanID:       conf.VxlanID,
		MTU:           conf.MTU,
	}
	vxlanIface, err := vxlan.SetupVxlan(vxlanConfig)
	if err != nil {
		return fmt.Errorf("failed to setup VXLAN: %v", err)
	}

	// Parse subnet
	_, subnet, err := net.ParseCIDR(conf.Subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %v", err)
	}

	// Configure VXLAN network
	if err := vxlan.ConfigureVxlanNetwork(vxlanIface, subnet); err != nil {
		return fmt.Errorf("failed to configure VXLAN network: %v", err)
	}

	// Initialize IPAM
	ipamConfig := &ipam.Config{
		Subnet:  conf.Subnet,
		Gateway: conf.Gateway,
		DataDir: conf.DataDir,
	}
	ipamInstance, err := ipam.New(ipamConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize IPAM: %v", err)
	}

	// Allocate IP for container
	containerIP, err := ipamInstance.Allocate(args.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to allocate IP: %v", err)
	}

	// Create veth pair
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	hostVeth, containerVeth, err := ip.SetupVeth(args.IfName, conf.MTU, "", netns)
	if err != nil {
		return fmt.Errorf("failed to setup veth pair: %v", err)
	}

	// Configure container network namespace
	err = ns.WithNetNSPath(args.Netns, func(netns ns.NetNS) error {
		// Get container veth
		link, err := netlink.LinkByName(args.IfName)
		if err != nil {
			return fmt.Errorf("failed to get container veth: %v", err)
		}

		// Add IP address to container veth
		addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   containerIP,
				Mask: subnet.Mask,
			},
		}
		if err := netlink.AddrAdd(link, addr); err != nil {
			return fmt.Errorf("failed to add IP address to container veth: %v", err)
		}

		// Set container veth up
		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("failed to set container veth up: %v", err)
		}

		// Add default route to container
		gateway := net.ParseIP(conf.Gateway)
		if gateway == nil {
			return fmt.Errorf("invalid gateway IP: %s", conf.Gateway)
		}
		defaultRoute := &netlink.Route{
			LinkIndex: link.Attrs().Index,
			Gw:        gateway,
			Dst:       nil, // Default route
		}
		if err := netlink.RouteAdd(defaultRoute); err != nil {
			return fmt.Errorf("failed to add default route: %v", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Connect host veth to VXLAN bridge
	hostLink, err := netlink.LinkByName(hostVeth.Name)
	if err != nil {
		return fmt.Errorf("failed to get host veth: %v", err)
	}
	if err := netlink.LinkSetMaster(hostLink, vxlanIface); err != nil {
		return fmt.Errorf("failed to connect host veth to VXLAN: %v", err)
	}

	// Prepare result
	result := &current.Result{
		CNIVersion: conf.CNIVersion,
		Interfaces: []*current.Interface{
			{
				Name:    args.IfName,
				Mac:     containerVeth.HardwareAddr.String(),
				Sandbox: args.Netns,
			},
			{
				Name: hostVeth.Name,
				Mac:  hostVeth.HardwareAddr.String(),
			},
			{
				Name: vxlanIface.Attrs().Name,
				Mac:  vxlanIface.Attrs().HardwareAddr.String(),
			},
		},
		IPs: []*current.IPConfig{
			{
				Interface: current.Int(0),
				Address: net.IPNet{
					IP:   containerIP,
					Mask: subnet.Mask,
				},
				Gateway: net.ParseIP(conf.Gateway),
			},
		},
	}

	return types.PrintResult(result, conf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	// Parse network configuration
	conf := &PluginConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse network configuration: %v", err)
	}

	// Initialize IPAM
	ipamConfig := &ipam.Config{
		Subnet:  conf.Subnet,
		Gateway: conf.Gateway,
		DataDir: conf.DataDir,
	}
	ipamInstance, err := ipam.New(ipamConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize IPAM: %v", err)
	}

	// Release IP
	if err := ipamInstance.Release(args.ContainerID); err != nil {
		return fmt.Errorf("failed to release IP: %v", err)
	}

	// Remove veth pair
	if args.Netns != "" {
		_, err := ip.DelLinkByNameAddr(args.IfName)
		if err != nil {
			return err
		}
	}

	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	// Parse network configuration
	conf := &PluginConf{}
	if err := json.Unmarshal(args.StdinData, conf); err != nil {
		return fmt.Errorf("failed to parse network configuration: %v", err)
	}

	// Check if VXLAN interface exists
	vxlanName := fmt.Sprintf("vxlan%d", conf.VxlanID)
	_, err := netlink.LinkByName(vxlanName)
	if err != nil {
		return fmt.Errorf("VXLAN interface %s not found: %v", vxlanName, err)
	}

	// Check container network namespace
	err = ns.WithNetNSPath(args.Netns, func(netns ns.NetNS) error {
		// Check if container interface exists
		link, err := netlink.LinkByName(args.IfName)
		if err != nil {
			return fmt.Errorf("container interface %s not found: %v", args.IfName, err)
		}

		// Check if container interface is up
		if link.Attrs().Flags&net.FlagUp == 0 {
			return fmt.Errorf("container interface %s is down", args.IfName)
		}

		// Check if container has an IP address
		addrs, err := netlink.AddrList(link, unix.AF_INET)
		if err != nil {
			return fmt.Errorf("failed to get addresses for container interface: %v", err)
		}
		if len(addrs) == 0 {
			return fmt.Errorf("container interface %s has no IPv4 address", args.IfName)
		}

		// Check if container has a default route
		routes, err := netlink.RouteList(link, unix.AF_INET)
		if err != nil {
			return fmt.Errorf("failed to get routes for container interface: %v", err)
		}
		hasDefaultRoute := false
		for _, route := range routes {
			if route.Dst == nil {
				hasDefaultRoute = true
				break
			}
		}
		if !hasDefaultRoute {
			return fmt.Errorf("container interface %s has no default route", args.IfName)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
