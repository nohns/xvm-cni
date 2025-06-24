//go:build linux
// +build linux

package vxlan

import (
	"net"
	"os"
	"testing"

	"github.com/vishvananda/netlink"
)

// findTestInterface finds a suitable network interface for testing
func findTestInterface(t *testing.T) string {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Failed to get network interfaces: %v", err)
	}

	for _, iface := range interfaces {
		// Skip loopback and non-up interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Check if interface has an IPv4 address
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return iface.Name
			}
		}
	}

	return ""
}

func TestSetupVxlan(t *testing.T) {
	// Skip test if not running as root
	if os.Geteuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	// Find a suitable interface for testing
	testInterface := findTestInterface(t)
	if testInterface == "" {
		t.Skip("No suitable interface found for testing")
	}

	// Test VXLAN setup
	config := &VxlanConfig{
		HostInterface: testInterface,
		VxlanID:       99, // Use a high ID to avoid conflicts
		MTU:           1500,
	}

	// Setup VXLAN
	if _, err := SetupVxlan(config); err != nil {
		t.Fatalf("Failed to setup VXLAN: %v", err)
	}

	// Verify VXLAN interface was created
	vxlanName := "vxlan99"
	link, err := netlink.LinkByName(vxlanName)
	if err != nil {
		t.Fatalf("VXLAN interface not found: %v", err)
	}

	// Verify VXLAN properties
	if link.Type() != "vxlan" {
		t.Fatalf("Expected interface type 'vxlan', got '%s'", link.Type())
	}

	vxlanLink, ok := link.(*netlink.Vxlan)
	if !ok {
		t.Fatalf("Failed to convert link to VXLAN")
	}

	if vxlanLink.VxlanId != config.VxlanID {
		t.Fatalf("Expected VXLAN ID %d, got %d", config.VxlanID, vxlanLink.VxlanId)
	}

	// Clean up
	if err := CleanupVxlan(config.VxlanID); err != nil {
		t.Fatalf("Failed to cleanup VXLAN: %v", err)
	}

	// Verify VXLAN interface was removed
	if _, err := netlink.LinkByName(vxlanName); err == nil {
		t.Fatalf("VXLAN interface still exists after cleanup")
	}
}
