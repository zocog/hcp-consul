#!/usr/bin/env bash
#
# This script generates tag-sets that can be used as runs-on: values to select runners.
# As a special case, this script also sets the `enterprise` output.  This is a convenience
# to avoid workflows needing a separate job for that one parameter selected on the same
# condition as the runner tags.

set -euo pipefail

case "$GITHUB_REPOSITORY" in
    *-enterprise)
        # shellcheck disable=SC2129
        echo 'compute-small-linux=["self-hosted", "linux", "small"]' >> "$GITHUB_OUTPUT"
        echo 'compute-medium-linux=["self-hosted", "linux", "medium"]' >> "$GITHUB_OUTPUT"
        echo 'compute-large-linux=["self-hosted", "linux", "large"]' >> "$GITHUB_OUTPUT"
        echo 'compute-xl-linux=["self-hosted", "ondemand", "linux", "type=m5.2xlarge"]' >> "$GITHUB_OUTPUT"
        echo 'compute-macos=["self-hosted", "ondemand", "os=macos"]' >> "$GITHUB_OUTPUT"
        echo 'compute-linux-standard=["ubuntu-latest"]' >> "$GITHUB_OUTPUT"
        echo 'compute-macos-standard=["macos-latest"]' >> "$GITHUB_OUTPUT"
        echo 'enterprise=1' >> "$GITHUB_OUTPUT"
        ;;
    *)
        # shellcheck disable=SC2129
        echo 'compute-small-linux=["custom", "linux", "small"]' >> "$GITHUB_OUTPUT"
        echo 'compute-medium-linux=["custom", "linux", "medium"]' >> "$GITHUB_OUTPUT"
        echo 'compute-large-linux=["custom", "linux", "large"]' >> "$GITHUB_OUTPUT"
        echo 'compute-xl-linux=["custom", "linux", "xl"]' >> "$GITHUB_OUTPUT"
        echo 'compute-linux-standard=["ubuntu-latest"]' >> "$GITHUB_OUTPUT"
        echo 'compute-macos-standard=["macos-latest"]' >> "$GITHUB_OUTPUT"
        echo 'enterprise=' >> "$GITHUB_OUTPUT"
        ;;
esac
