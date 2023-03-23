#!/usr/bin/env bash
set -euo pipefail
GITHUB_OUTPUT=
# set matrix var to list of unique packages containing tests
matrix="$(go list -json="ImportPath,TestGoFiles" ./... | jq --compact-output '. | select(.TestGoFiles != null) | .ImportPath' | jq --slurp --compact-output '.'))"

# splitMatrix=$("jq -nc '_nwise($matrix;3)'")
echo "matrix=${matrix}" | tee -a "${GITHUB_OUTPUT}"
