#!/bin/bash

# this is meant to be run inside a container that supports the apt package manager (eg. debian, ubuntu)
function main {
  local pkg=${1:-}
  local version=${2:-}
  local platform

  if [[ -z "${pkg}" ]]; then
    echo "ERROR: pkg argument is required"
    exit 1
  fi

  if [[ -z "${version}" ]]; then
    echo "ERROR: version argument is required"
    exit 1
  fi

  apt update && apt install -y software-properties-common wget gpg
  wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor > /usr/share/keyrings/hashicorp-archive-keyring.gpg

  echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" > /etc/apt/sources.list.d/hashicorp.list
  echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) test" >> /etc/apt/sources.list.d/hashicorp.list

  apt-get update && apt show "${pkg}"
  apt list -a "${pkg}" | grep ${version} 
}

main "$@"
