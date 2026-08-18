#!/bin/sh
set -eu

# Build mssql-to-csv for the current platform with version info from git.
# Usage: ./build.sh [output-path]

b_output="${1:-mssql-to-csv}"

b_version=$(git describe --tags --always --dirty 2> /dev/null || echo 0.0.0-dev)
b_version=$(echo "$b_version" | sed 's/^v//')
b_commit=$(git rev-parse --short HEAD 2> /dev/null || echo 0000000)
b_date=$(date "+%F %T %Z")

CGO_ENABLED=0 go build \
	-ldflags "-s -w -X main.version=${b_version} -X main.commit=${b_commit} -X 'main.date=${b_date}'" \
	-o "$b_output" \
	.

echo "${b_output} v${b_version}"
