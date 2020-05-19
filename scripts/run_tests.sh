#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..
cd $ROOT_DIR

tmpdir="$(mktemp -d)"

function cleanup() {
    for m in $(mount | grep "on $tmpdir" | awk '{print $3}'); do
        umount "$m"
    done
    rm -rf $tmpdir
}
trap cleanup EXIT

function tmpd() {
    # Remove directories from previous test cases.
    find $tmpdir -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} \; > /dev/null 2>&1
    echo "$(mktemp -d -p $tmpdir)"
}

function tmpf() {
    local f="$(mktemp -p $tmpdir)"
    rm "$f"
    echo $f
}

# Basic checks.
./tosi -image bitnami/tomcat:8.5.31 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
./tosi -image library/nginx:1.7.9 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
./tosi -image library/nginx:latest -saveconfig "$(tmpf)" -extractto "$(tmpd)"
./tosi -image library/ruby -saveconfig "$(tmpf)" -extractto "$(tmpd)"
./tosi -image library/alpine:3.6 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
./tosi -image library/alpine@sha256:6a92cd1fcdc8d8cdec60f33dda4db2cb1fcdcacf3410a8e05b3741f44a9b5998 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
./tosi -image library/alpine:latest@sha256:6a92cd1fcdc8d8cdec60f33dda4db2cb1fcdcacf3410a8e05b3741f44a9b5998 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
./tosi -image k8s.gcr.io/k8s-dns-kube-dns-amd64:1.14.10 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
# Manifest is v1+prettyjws.
./tosi -image k8s.gcr.io/redis:e2e -saveconfig "$(tmpf)" -extractto "$(tmpd)"
# One layer contains a directory without creating parent directory first.
./tosi -image jenkinsxio/jx:2.0.22 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
# Manifest that requires per-layer whiteouts.
./tosi -image gcr.io/google-samples/gb-frontend:v4 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
# Registry that does not support pings.
#./tosi -image quay.io/quay/redis -saveconfig "$(tmpf)" -extractto "$(tmpd)"
# Old-style deprecated URL parameter.
./tosi -url https://quay.io -image calico/cni:v3.4.0 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
# A layer creates a file that overwrites a symlink from a previous layer.
./tosi -image gcr.io/google-containers/conformance:v1.17.3 -saveconfig "$(tmpf)" -extractto "$(tmpd)"
# Create overlayfs.
rootfs="$(tmpd)"
./tosi -image library/ubuntu -mount "$rootfs"
[ "$(ls -l $rootfs | wc -l)" -gt 5 ]
# Use cached layers.
./tosi -image library/ubuntu
