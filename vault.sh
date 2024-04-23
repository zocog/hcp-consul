#!/bin/bash

set -euo pipefail

unset CDPATH

cd "$(dirname "$0")"

readonly COMMAND="${1:-}"

export VAULT_ADDR='http://127.0.0.1:8200'

vault server -config=vault.hcl
