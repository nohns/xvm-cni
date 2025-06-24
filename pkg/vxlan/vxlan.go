//go:build linux
// +build linux

package vxlan

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const (
	// DefaultVxlanPort is the default VXLAN UDP port
	DefaultVxlanPort = 8472
	// DefaultVxlanVNI is the default VXLAN Network Identifier
	DefaultVxlanVNI = 10
	// DefaultMTU is the default MTU for VXLAN interfaces
	DefaultMTU = 1500
)

// VxlanConfig holds the configuration for a VXLAN network
type VxlanConfig struct {
	HostInterface string
	VxlanID       int
	MTU           int
}

// SetupVxlan creates a VXLAN interface and configures it
func SetupVxlan(config *VxlanConfig) (*netlink.Vxlan, error) {
	// Get the host interface
	hostIface, err := netlink.LinkByName(config.HostInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to get host interface %s: %v", config.HostInterface, err)
	}

	// Get the IP address of the host interface
	addrs, err := netlink.AddrList(hostIface, unix.AF_INET)
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses for interface %s: %v", config.HostInterface, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no IPv4 address found on interface %s", config.HostInterface)
	}
	hostIP := addrs[0].IP

	// Create VXLAN interface
	vxlanName := fmt.Sprintf("vxlan%d", config.VxlanID)
	vxlan := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name:   vxlanName,
			MTU:    config.MTU,
			TxQLen: 1000,
		},
		VxlanId:      config.VxlanID,
		VtepDevIndex: hostIface.Attrs().Index,
		SrcAddr:      hostIP,
		Port:         DefaultVxlanPort,
		Learning:     true,
		GBP:          false,
		// Enable multicast for discovery
		Group: net.ParseIP("239.1.1.1"), // Multicast group IP
	}

	// Check if the VXLAN interface already exists
	existing, err := netlink.LinkByName(vxlanName)
	if err == nil {
		// If it exists, delete it first
		if err := netlink.LinkDel(existing); err != nil {
			return nil, fmt.Errorf("failed to delete existing VXLAN interface: %v", err)
		}
	}

	// Add the VXLAN interface
	if err := netlink.LinkAdd(vxlan); err != nil {
		return nil, fmt.Errorf("failed to create VXLAN interface: %v", err)
	}

	// Set the VXLAN interface up
	if err := netlink.LinkSetUp(vxlan); err != nil {
		return nil, fmt.Errorf("failed to set VXLAN interface up: %v", err)
	}

	return vxlan, nil
}

// CleanupVxlan removes the VXLAN interface
func CleanupVxlan(vxlanID int) error {
	vxlanName := fmt.Sprintf("vxlan%d", vxlanID)
	link, err := netlink.LinkByName(vxlanName)
	if err != nil {
		// If the interface doesn't exist, that's fine
		return nil
	}

	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete VXLAN interface: %v", err)
	}

	return nil
}

// ConfigureVxlanNetwork sets up the network configuration for the VXLAN
func ConfigureVxlanNetwork(vxlan *netlink.Vxlan, subnet *net.IPNet) error {
	// Add an IP address to the VXLAN interface
	addr := &netlink.Addr{
		IPNet: subnet,
	}
	if err := netlink.AddrAdd(vxlan, addr); err != nil {
		return fmt.Errorf("failed to add IP address to VXLAN interface: %v", err)
	}

	return nil
}
