#!/bin/bash

set -euo pipefail

# verify_docker_post.sh verifies images that already exist in an external repository
# are valid. This is meant to be run post-release to verify images were published correctly.
# Depending on the channel and product, the script knows where to look for the image.


function usage {
    echo "usage:"
    echo "  verify_docker_post.sh <product> <sha> <version> <channel>"
    echo
    echo "example:"
    echo "  verify_docker_post.sh consul-enterprise 0a4743c5d48e458cbb15d424113080539ca010bf v1.12.2+ent staging"

}

PRODUCT_OSS=consul
PRODUCT_ENT=consul-enterprise
CHANNEL_STAGING=staging
CHANNEL_PRODUCTION=production
DOCKER_HOST_STAGING=crt-core-staging-docker-local.artifactory.hashicorp.engineering
ARCH=( "386" "amd64" "arm64" "armv6" )

# Arguments:
#   $1 - product (either consul or consul-enterprise)
#   $2 - git sha (eg. 19041f202c952a25c5d612a3664e1bdac3c91129)
#   $4 - version to match against, note: should not have 'v' prefix. (eg. 1.13.0-dev, 1.12.2+ent)
#   $3 - channel (either staging or production)
function main {
  local product="${1:-}"
  local sha="${2:-}"
  local version="${3:-}"
  local channel="${4:-}"
  local uri

  if [[ -z "${product}" ]]; then
    echo "ERROR: product argument is required"
    usage
    exit 1
  fi

  if [[ -z "${sha}" ]]; then
    echo "ERROR: sha argument is required"
    usage
    exit 1
  fi

  if [[ -z "${version}" ]]; then
    echo "ERROR: version argument is required"
    usage
    exit 1
  fi

  if [[ -z "${channel}" ]]; then
    echo "ERROR: channel argument is required"
    usage
    exit 1
  fi

  if [[ ! "${product}" = $PRODUCT_OSS ]] && [[ ! "${product}" = $PRODUCT_ENT ]]; then
    echo "ERROR: product must be one of $PRODUCT_OSS, $PRODUCT_ENT, got ${product}"
    usage
    exit 1
  fi

  if [[ ! "${channel}" = $CHANNEL_STAGING ]] && [[ ! "${channel}" = $CHANNEL_PRODUCTION ]]; then
    echo "ERROR: channel must be one of $CHANNEL_STAGING, $CHANNEL_PRODUCTION, got ${channel}"
    usage
    exit 1
  fi

  # construct the URI
  # examples for staging:
  #   crt-core-staging-docker-local.artifactory.hashicorp.engineering/consul/default:1.12.2_19041f202c952a25c5d612a3664e1bdac3c91129-linux-amd64
  #   crt-core-staging-docker-local.artifactory.hashicorp.engineering/consul-enterprise/default:1.12.2-ent_0a4743c5d48e458cbb15d424113080539ca010bf-linux-arm64
  # examples for production:
  #   hashicorp/consul:1.12
  #   hashicorp/consul-enterprise:1.12.2-ent
  if [[ "${channel}" = $CHANNEL_PRODUCTION ]]; then
    for arch in ${ARCH[@]}; do
      uri="hashicorp/${product}:${version/+/-}"
      smoke_test "${uri}" $(platform_for_arch "${arch}")
    done
  elif [[ "${channel}" = "$CHANNEL_STAGING" ]]; then
    for arch in ${ARCH[@]}; do
      uri="${DOCKER_HOST_STAGING}/${product}/default:${version/+/-}_${sha}-linux-${arch}" 
      smoke_test "${uri}" $(platform_for_arch "${arch}")
    done
  fi
}

function smoke_test {
  local uri="${1:-}"
  local platform="${2:-}"

  # ensure we have the right image for the platform being tested
  # docker image rm -f "${uri}"

  echo "Pulling image for ${platform}"
  # docker pull --platform ${platform} "${uri}"

  echo "Running smoke test for ${uri} / ${arch}"
  got_version="$( awk '{print $2}' <(head -n1 <(docker run --platform ${platform} "${uri}" version)) )"
  if [[ "${got_version}" != "v${version}" ]]; then
    echo "Test FAILED"
    echo "Got: ${got_version}, Want: v${version}"
  else
    echo "Test PASSED"
  fi
}

function platform_for_arch {
  local arch="${1:-}"
  case "$arch" in
    "386")
      echo "linux/386"
      ;;

    "amd64")
      echo "linux/amd64"
      ;;

    "arm64")
      echo "linux/arm64"
      ;;

    "armv6")
      echo "linux/arm/v6"
      ;;
  esac
}

main "$@"
