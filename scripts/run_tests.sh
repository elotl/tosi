#!/bin/bash

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..
cd $ROOT_DIR
echo ">>> git diff-index:"
git diff-index --quiet HEAD
echo ">>> end of git diff-index"
make
go test ./...

compare_rootfs() {
    rootfs="$1"
    pushd "$rootfs"
    find . -exec ls -d -l {} \; | awk '{ s = ""; for (i = 9; i <= NF; i++) s = s $i " "; print s }' | sort > /tmp/files1.txt
    cd ..
    mkdir -p check
    cd check
    tar xvzf ../*.tar.gz
    cd ROOTFS
    find . -exec ls -d -l {} \; | awk '{ s = ""; for (i = 9; i <= NF; i++) s = s $i " "; print s }' | sort > /tmp/files2.txt
    cmp /tmp/files1.txt /tmp/files2.txt
    popd
}

echo -n > /tmp/tosi.log

./tosi -image bitnami/tomcat:8.5.31 >> /tmp/tosi.log 2>&1
./tosi -image library/nginx:1.7.9 >> /tmp/tosi.log 2>&1
./tosi -image library/nginx:latest >> /tmp/tosi.log 2>&1
./tosi -image k8s.gcr.io/k8s-dns-kube-dns-amd64:1.14.10 >> /tmp/tosi.log 2>&1
# Manifest is v1+prettyjws.
./tosi -image k8s.gcr.io/redis:e2e >> /tmp/tosi.log 2>&1
# Manifest that requires per-layer whiteouts.
./tosi -url https://gcr.io -image google-samples/gb-frontend:v4 >> /tmp/tosi.log 2>&1
# Registry that does not support pings.
./tosi -url https://quay.io -image quay/redis >> /tmp/tosi.log 2>&1

./tosi -image library/alpine:3.6 >> /tmp/tosi.log 2>&1
compare_rootfs "/tmp/tosi/packages/library-alpine-3.6/3.6/ROOTFS"

./tosi -image library/ruby >> /tmp/tosi.log 2>&1
compare_rootfs "/tmp/tosi/packages/library-ruby/latest/ROOTFS"
