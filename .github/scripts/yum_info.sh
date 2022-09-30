#!/bin/bash

# this is meant to be run inside a container that supports the yum package manager (eg. RHEL, CentOS)
function main {
  local pkg=${1:-}
  local version=${2:-}
  local repo=${3:-}

  if [[ -z "${pkg}" ]]; then
    echo "ERROR: pkg argument is required"
    exit 1
  fi

  if [[ -z "${version}" ]]; then
    echo "ERROR: version argument is required"
    exit 1
  fi

  if [[ -z "${repo}" ]]; then
    repo=https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
  fi

  yum install -y yum-utils
  yum-config-manager --add-repo "${repo}" 

  # enable test channel so our pre-releases are included
  sed -i 's/enabled=0/enabled=1/g' /etc/yum.repos.d/hashicorp.repo

  # should show latest version
  yum info "${pkg}"

  yum --showduplicates list "${pkg}" | grep "${version}" 
}

main "$@"
