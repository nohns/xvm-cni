#!/bin/bash
set -e

# Script to cross-compile XVM CNI plugin for different architectures

# Default target is Linux/ARM64
TARGET_OS=${1:-linux}
TARGET_ARCH=${2:-arm64}
OUTPUT_NAME=${3:-xvm-cni}

echo "Cross-compiling for ${TARGET_OS}/${TARGET_ARCH}..."

# Set environment variables for cross-compilation
export GOOS=${TARGET_OS}
export GOARCH=${TARGET_ARCH}
export CGO_ENABLED=0

# Ensure bin directory exists
mkdir -p bin

# Build the binary
go build -o bin/${OUTPUT_NAME} main.go

# Verify the binary
echo "Verifying binary..."
file bin/${OUTPUT_NAME}

echo "Cross-compilation complete: bin/${OUTPUT_NAME}"
echo "Target: ${TARGET_OS}/${TARGET_ARCH}"
