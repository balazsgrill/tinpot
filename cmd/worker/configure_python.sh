#!/bin/bash
cd $(dirname "$0") 
set -e

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <3.11|3.12>"
    exit 1
fi

echo "Configuring Worker for Python $VERSION..."

# Go module versions for nhatthm/python
# These are approximate based on user usage. 
# We can use @latest matching the minor version pattern if tags exist
# User used 3.11.3 for python/v3 and 3.11.2 for cpy/v3.

if [ "$VERSION" == "3.11" ]; then
    go get go.nhat.io/python/v3@v3.11.3
    go get go.nhat.io/cpy/v3@v3.11.2
elif [ "$VERSION" == "3.12" ]; then
    go get go.nhat.io/python/v3@v3.12.0
    go get go.nhat.io/cpy/v3@v3.12.0
else
    echo "Unsupported Python version: $VERSION"
    exit 1
fi

go mod tidy
echo "Done. Worker configured for Python $VERSION."
