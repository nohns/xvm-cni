# Makefile for XVM CNI Plugin

# Default target
.PHONY: all
all: build

# Build target - runs the cross-compile script
.PHONY: build
build:
	@echo "Building XVM CNI plugin..."
	@./scripts/cross-compile.sh

# Clean target - removes built binaries
.PHONY: clean
clean:
	@echo "Cleaning up..."
	@rm -rf bin/*

# Install target - installs the plugin on the system
.PHONY: install
install: build
	@echo "Installing XVM CNI plugin..."
	@mkdir -p /etc/cni/net.d
	@mkdir -p /opt/cni/bin
	@cp bin/xvm-cni /opt/cni/bin/
	@cp examples/xvm-cni.conf /etc/cni/net.d/10-xvm.conf
	@echo "Installation complete!"
	@echo "Plugin installed to: /opt/cni/bin/xvm-cni"
	@echo "Configuration installed to: /etc/cni/net.d/10-xvm.conf"

# Help target
.PHONY: help
help:
	@echo "XVM CNI Plugin Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build    - Cross-compile the plugin (default target)"
	@echo "  install  - Install the plugin on the system (requires root privileges)"
	@echo "  clean    - Remove built binaries"
	@echo "  help     - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build                   - Build for default target (linux/arm64)"
	@echo "  TARGET_OS=linux TARGET_ARCH=amd64 make build - Build for linux/amd64"
	@echo "  OUTPUT_NAME=custom-name make build - Build with custom output name"
	@echo "  sudo make install            - Install the plugin (requires root privileges)"
