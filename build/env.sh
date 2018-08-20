#!/bin/sh

set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
entropydir="$workspace/src/github.com/entropyio"
if [ ! -L "$entropydir/go-entropy" ]; then
    mkdir -p "$entropydir"
    cd "$entropydir"
    ln -s ../../../../../. go-entropy
    cd "$root"
fi

# Set up the environment to use the workspace.
GOPATH="$workspace"
export GOPATH

# Run the command inside the workspace.
cd "$entropydir/go-entropy"
PWD="$entropydir/go-entropy"

# Launch the arguments with the configured environment.
exec "$@"
