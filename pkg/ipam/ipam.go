package ipam

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// IPAM represents the IP Address Management system
type IPAM struct {
	Subnet     *net.IPNet
	Gateway    net.IP
	Allocations map[string]net.IP
	mutex      sync.Mutex
	dataDir    string
}

// Config represents the IPAM configuration
type Config struct {
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
	DataDir string `json:"dataDir"`
}

// New creates a new IPAM instance
func New(config *Config) (*IPAM, error) {
	_, subnet, err := net.ParseCIDR(config.Subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet: %v", err)
	}

	gateway := net.ParseIP(config.Gateway)
	if gateway == nil {
		return nil, fmt.Errorf("invalid gateway IP: %s", config.Gateway)
	}

	// Create data directory if it doesn't exist
	dataDir := config.DataDir
	if dataDir == "" {
		dataDir = "/var/lib/cni/xvm-cni"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	ipam := &IPAM{
		Subnet:     subnet,
		Gateway:    gateway,
		Allocations: make(map[string]net.IP),
		dataDir:    dataDir,
	}

	// Load existing allocations
	if err := ipam.loadAllocations(); err != nil {
		return nil, err
	}

	return ipam, nil
}

// Allocate allocates an IP address for the given container ID
func (i *IPAM) Allocate(containerID string) (net.IP, error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	// Check if container already has an allocation
	if ip, ok := i.Allocations[containerID]; ok {
		return ip, nil
	}

	// Find an available IP
	ip, err := i.findAvailableIP()
	if err != nil {
		return nil, err
	}

	// Save the allocation
	i.Allocations[containerID] = ip
	if err := i.saveAllocations(); err != nil {
		return nil, err
	}

	return ip, nil
}

// Release releases the IP address for the given container ID
func (i *IPAM) Release(containerID string) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	// Check if container has an allocation
	if _, ok := i.Allocations[containerID]; !ok {
		return nil // Nothing to release
	}

	// Remove the allocation
	delete(i.Allocations, containerID)
	if err := i.saveAllocations(); err != nil {
		return err
	}

	return nil
}

// findAvailableIP finds an available IP address in the subnet
func (i *IPAM) findAvailableIP() (net.IP, error) {
	// Start from the first IP in the subnet
	ip := make(net.IP, len(i.Subnet.IP))
	copy(ip, i.Subnet.IP)

	// Increment to the first usable IP (network address + 1)
	inc(ip)

	// Skip the gateway IP
	if ip.Equal(i.Gateway) {
		inc(ip)
	}

	// Check each IP until we find an available one
	for {
		// Check if IP is in subnet
		if !i.Subnet.Contains(ip) {
			return nil, fmt.Errorf("no available IP addresses in subnet")
		}

		// Check if IP is already allocated
		allocated := false
		for _, allocatedIP := range i.Allocations {
			if ip.Equal(allocatedIP) {
				allocated = true
				break
			}
		}

		if !allocated {
			return ip, nil
		}

		// Try the next IP
		inc(ip)
		if ip.Equal(i.Gateway) {
			inc(ip)
		}
	}
}

// inc increments the IP address
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// loadAllocations loads the IP allocations from disk
func (i *IPAM) loadAllocations() error {
	file := filepath.Join(i.dataDir, "allocations.json")
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No allocations file yet
		}
		return fmt.Errorf("failed to read allocations file: %v", err)
	}

	allocations := make(map[string]string)
	if err := json.Unmarshal(data, &allocations); err != nil {
		return fmt.Errorf("failed to parse allocations file: %v", err)
	}

	// Convert string IPs to net.IP
	for id, ipStr := range allocations {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return fmt.Errorf("invalid IP address in allocations: %s", ipStr)
		}
		i.Allocations[id] = ip
	}

	return nil
}

// saveAllocations saves the IP allocations to disk
func (i *IPAM) saveAllocations() error {
	// Convert net.IP to string for JSON serialization
	allocations := make(map[string]string)
	for id, ip := range i.Allocations {
		allocations[id] = ip.String()
	}

	data, err := json.Marshal(allocations)
	if err != nil {
		return fmt.Errorf("failed to marshal allocations: %v", err)
	}

	file := filepath.Join(i.dataDir, "allocations.json")
	if err := os.WriteFile(file, data, 0644); err != nil {
		return fmt.Errorf("failed to write allocations file: %v", err)
	}

	return nil
}