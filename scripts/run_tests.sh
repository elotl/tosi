#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..
cd $ROOT_DIR

tmpdir="$(mktemp -d)"

function cleanup() {
    rm -rf $tmpdir
}
trap cleanup EXIT

echo -n > /tmp/tosi.log

./tosi -image bitnami/tomcat:8.5.31 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
./tosi -image library/nginx:1.7.9 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
./tosi -image library/nginx:latest -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
./tosi -image library/ruby -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
./tosi -image library/alpine:3.6 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
./tosi -image library/alpine@sha256:6a92cd1fcdc8d8cdec60f33dda4db2cb1fcdcacf3410a8e05b3741f44a9b5998 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
./tosi -image library/alpine:latest@sha256:6a92cd1fcdc8d8cdec60f33dda4db2cb1fcdcacf3410a8e05b3741f44a9b5998 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
./tosi -image k8s.gcr.io/k8s-dns-kube-dns-amd64:1.14.10 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
# Manifest is v1+prettyjws.
./tosi -image k8s.gcr.io/redis:e2e -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
# One layer contains a directory without creating parent directory first.
./tosi -image jenkinsxio/jx:2.0.22 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
# Manifest that requires per-layer whiteouts.
./tosi -url https://gcr.io -image google-samples/gb-frontend:v4 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
# Registry that does not support pings.
./tosi -url https://quay.io -image quay/redis -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
# A layer creates a file that overwrites a symlink from a previous layer.
./tosi -url https://gcr.io/ -image google-containers/conformance:v1.17.3 -saveconfig /tmp/config -extractto "$(mktemp -d -p $tmpdir)" >> /tmp/tosi.log 2>&1
