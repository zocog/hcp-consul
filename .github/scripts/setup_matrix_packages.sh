#!/usr/bin/env bash
set -euo pipefail

array=$(go list -json="ImportPath,TestGoFiles" ./... | jq --compact-output '. | select(.TestGoFiles != null) | .ImportPath' | jq --slurp --compact-output '.')

g=4

def nwise($n):
 def _nwise:
   if length <= $n then . else .[0:$n] , (.[$n:]|_nwise) end;
 _nwise;

nwise(array)
