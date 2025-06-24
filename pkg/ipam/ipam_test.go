package ipam

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestIPAM(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := ioutil.TempDir("", "ipam-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create IPAM instance
	config := &Config{
		Subnet:  "10.244.0.0/24",
		Gateway: "10.244.0.1",
		DataDir: tempDir,
	}

	ipamInstance, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create IPAM instance: %v", err)
	}

	// Test IP allocation
	containerID1 := "container1"
	ip1, err := ipamInstance.Allocate(containerID1)
	if err != nil {
		t.Fatalf("Failed to allocate IP for container1: %v", err)
	}

	// Verify IP is in subnet
	_, subnet, _ := net.ParseCIDR(config.Subnet)
	if !subnet.Contains(ip1) {
		t.Fatalf("Allocated IP %s is not in subnet %s", ip1, config.Subnet)
	}

	// Verify IP is not the gateway
	gateway := net.ParseIP(config.Gateway)
	if ip1.Equal(gateway) {
		t.Fatalf("Allocated IP %s is the gateway", ip1)
	}

	// Test second allocation
	containerID2 := "container2"
	ip2, err := ipamInstance.Allocate(containerID2)
	if err != nil {
		t.Fatalf("Failed to allocate IP for container2: %v", err)
	}

	// Verify second IP is different from first
	if ip2.Equal(ip1) {
		t.Fatalf("Second allocated IP %s is the same as first %s", ip2, ip1)
	}

	// Test allocation persistence
	allocFile := filepath.Join(tempDir, "allocations.json")
	if _, err := os.Stat(allocFile); os.IsNotExist(err) {
		t.Fatalf("Allocations file was not created")
	}

	// Create a new IPAM instance to test loading allocations
	ipamInstance2, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create second IPAM instance: %v", err)
	}

	// Verify allocations were loaded
	if len(ipamInstance2.Allocations) != 2 {
		t.Fatalf("Expected 2 allocations, got %d", len(ipamInstance2.Allocations))
	}

	// Verify container1 allocation
	if !ipamInstance2.Allocations[containerID1].Equal(ip1) {
		t.Fatalf("Loaded IP for container1 %s doesn't match original %s", 
			ipamInstance2.Allocations[containerID1], ip1)
	}

	// Test IP release
	if err := ipamInstance2.Release(containerID1); err != nil {
		t.Fatalf("Failed to release IP for container1: %v", err)
	}

	// Verify container1 allocation was removed
	if _, ok := ipamInstance2.Allocations[containerID1]; ok {
		t.Fatalf("Allocation for container1 was not removed")
	}

	// Test allocating a new IP after release
	containerID3 := "container3"
	ip3, err := ipamInstance2.Allocate(containerID3)
	if err != nil {
		t.Fatalf("Failed to allocate IP for container3: %v", err)
	}

	// Verify new IP is not the same as container2
	if ip3.Equal(ip2) {
		t.Fatalf("Third allocated IP %s is the same as second %s", ip3, ip2)
	}

	// Verify we can reuse the released IP
	if !ip3.Equal(ip1) {
		t.Logf("Note: Released IP was not reused, this is acceptable but not optimal")
	}
}