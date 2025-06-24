# XVM CNI (Cross-Virtual Machine CNI)

This system implements a CNI plugin developed in Go that creates a shared VXLAN network for containers across multiple hosts.

## How it works

When the XVM CNI plugin is installed and used in a CNI configuration, it:

1. Creates a shared VXLAN network for containers over an existing host L3-network defined by a given host interface and assigned IP.
2. Uses multi-cast broadcasting for discovery of other hosts on the VXLAN.
3. Manages IP address allocation for containers using a simple IPAM system.
4. Sets up container networking with proper routes and connectivity.

## Installation

### Prerequisites

- Go 1.16 or later
- Linux with VXLAN support (kernel 3.7+)
- Root privileges (for network operations)

### Building the plugin

```bash
# Clone the repository
git clone https://github.com/yourusername/xvm-cni.git
cd xvm-cni

# Build the plugin
go build -o bin/xvm-cni main.go

# Cross-compile for Linux/ARM64 (for deployment on ARM-based systems)
./scripts/cross-compile.sh

# Cross-compile for a specific OS/architecture
./scripts/cross-compile.sh linux amd64 xvm-cni-amd64
```

### Installing the plugin

#### Method 1: Using make (recommended)

```bash
# Build and install the plugin (requires root privileges)
sudo make install
```

#### Method 2: Manual installation

```bash
# Create CNI configuration directory if it doesn't exist
sudo mkdir -p /etc/cni/net.d

# Copy the plugin binary to the CNI bin directory
sudo cp bin/xvm-cni /opt/cni/bin/

# Create a CNI configuration file
sudo cp examples/xvm-cni.conf /etc/cni/net.d/10-xvm.conf
```

## Configuration

The plugin requires a CNI configuration file. Here's an example:

```json
{
  "cniVersion": "1.0.0",
  "name": "xvm-network",
  "type": "xvm-cni",
  "hostInterface": "eth0",
  "vxlanID": 10,
  "mtu": 1500,
  "subnet": "10.244.0.0/16",
  "gateway": "10.244.0.1",
  "dataDir": "/var/lib/cni/xvm-cni"
}
```

### Configuration Parameters

- `cniVersion`: CNI specification version
- `name`: Network name
- `type`: Must be "xvm-cni"
- `hostInterface`: The host interface to use for VXLAN traffic
- `vxlanID`: VXLAN network identifier (1-16777215)
- `mtu`: Maximum Transmission Unit for the VXLAN interface
- `subnet`: Subnet for container IPs (CIDR notation)
- `gateway`: Gateway IP for the container network
- `dataDir`: Directory to store IPAM data

## Target Machines

All VMs can be presumed to be running Ubuntu Linux.

## Testing

### Unit Tests

The project includes tests for the VXLAN and IPAM components:

```bash
# Run tests (requires root privileges for VXLAN tests)
sudo go test ./pkg/...
```

### Integration Testing

A test script is provided to verify the plugin's functionality by creating a network namespace and configuring it with the plugin:

```bash
# Run the integration test (requires root privileges)
sudo ./scripts/test-plugin.sh
```

This script:
1. Creates a network namespace
2. Configures it using the XVM CNI plugin
3. Verifies the interfaces are correctly set up
4. Tests connectivity
5. Cleans up resources when done

The test script is useful for verifying that the plugin works correctly in a real environment.

## Troubleshooting

### Common Issues

1. **Plugin fails to create VXLAN interface**
   - Ensure the host interface exists and has an IPv4 address
   - Check if the kernel supports VXLAN (modprobe vxlan)
   - Verify you have sufficient permissions

2. **Containers cannot communicate across hosts**
   - Ensure multicast traffic is allowed between hosts
   - Check if the VXLAN interfaces are properly configured on all hosts
   - Verify the subnet configuration is consistent across all hosts

### Logs

The plugin logs to stderr, which is captured by the container runtime.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
