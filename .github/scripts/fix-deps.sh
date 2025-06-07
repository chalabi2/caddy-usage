#!/bin/bash

# Fix dependency issues for CI
set -e

echo "Cleaning and updating Go dependencies..."

# Clean module cache
go clean -modcache

# Remove go.sum to force regeneration
rm -f go.sum

# Download and verify all dependencies
go mod download
go mod tidy
go mod verify

# Ensure all dependencies are properly downloaded
go list -m all

echo "Dependencies fixed successfully!" 