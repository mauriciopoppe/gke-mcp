#!/bin/bash

set -euxo pipefail

rm -rf ~/.gemini/extensions/gke-mcp
go build -o gke-mcp .
./gke-mcp install gemini-cli --developer
echo "done!"
