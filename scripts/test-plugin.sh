#!/bin/bash
set -e

# Test script for XVM CNI plugin
# This script creates a network namespace and uses the XVM CNI plugin to configure it

# Check if running as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root"
    exit 1
fi

# Ensure the CNI plugin is built
if [ ! -f "bin/xvm-cni" ]; then
    echo "Building XVM CNI plugin..."
    make build
fi

# Create a temporary directory for test files
TEMP_DIR=$(mktemp -d)
echo "Using temporary directory: $TEMP_DIR"

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    # Delete the network namespace
    ip netns del xvm-test-ns 2>/dev/null || true
    # Remove temporary directory
    rm -rf "$TEMP_DIR"
    echo "Cleanup complete"
}

# Set up trap to ensure cleanup on exit
trap cleanup EXIT

# Detect the default network interface
DEFAULT_IFACE=$(ip route | grep default | awk '{print $5}')
if [ -z "$DEFAULT_IFACE" ]; then
    echo "Could not detect default network interface"
    exit 1
fi
echo "Using network interface: $DEFAULT_IFACE"

# Create a test network namespace
echo "Creating network namespace: xvm-test-ns"
ip netns add xvm-test-ns

# Create a CNI configuration file
CNI_CONF="$TEMP_DIR/xvm-test.conf"
cat > "$CNI_CONF" << EOF
{
  "cniVersion": "1.0.0",
  "name": "xvm-test-network",
  "type": "xvm-cni",
  "hostInterface": "$DEFAULT_IFACE",
  "vxlanID": 42,
  "mtu": 1500,
  "subnet": "10.22.0.0/16",
  "gateway": "10.22.0.1",
  "dataDir": "$TEMP_DIR/cni-data"
}
EOF
echo "Created CNI configuration at: $CNI_CONF"

# Echo the configuration in an indented way
echo "Configuration:"
if command -v jq &> /dev/null; then
    # If jq is available, use it for pretty printing
    jq . "$CNI_CONF"
else
    # Otherwise, use cat with indentation
    echo "$(cat "$CNI_CONF" | sed 's/^/    /')"
fi

# Create the netconf data to pass to the CNI plugin
NETCONF=$(cat "$CNI_CONF")

# Create the command arguments
CONTAINER_ID="test-container-$(date +%s)"
IFNAME="eth0"
NETNS="/var/run/netns/xvm-test-ns"

# Call the CNI plugin to add the container to the network
echo "Calling CNI plugin to add container to network..."
echo "$NETCONF" | CNI_COMMAND=ADD CNI_CONTAINERID="$CONTAINER_ID" CNI_NETNS="$NETNS" CNI_IFNAME="$IFNAME" CNI_PATH="$(pwd)/bin" ./bin/xvm-cni

# Verify the setup
echo "Verifying network setup..."

# Check if VXLAN interface exists
if ! ip link show vxlan42 &>/dev/null; then
    echo "ERROR: VXLAN interface vxlan42 not found"
    exit 1
fi
echo "VXLAN interface vxlan42 exists"

# Check if container interface exists and has IP
echo "Checking container interface..."
ip netns exec xvm-test-ns ip addr show "$IFNAME"

# Check if container has default route
echo "Checking container routes..."
ip netns exec xvm-test-ns ip route

# Test connectivity from container to host
echo "Testing connectivity from container to host..."
ip netns exec xvm-test-ns ping -c 3 10.22.0.1

# Call the CNI plugin to remove the container from the network
echo "Calling CNI plugin to remove container from network..."
echo "$NETCONF" | CNI_COMMAND=DEL CNI_CONTAINERID="$CONTAINER_ID" CNI_NETNS="$NETNS" CNI_IFNAME="$IFNAME" CNI_PATH="$(pwd)/bin" ./bin/xvm-cni

echo "Test completed successfully!"
