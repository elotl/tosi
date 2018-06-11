#!/bin/bash

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..
cd $ROOT_DIR
go build
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

./tosi -image library/alpine:3.6 >> /tmp/tosi.log 2>&1
compare_rootfs "/tmp/tosi/packages/library-alpine-3.6/3.6/ROOTFS"

./tosi -image library/ruby >> /tmp/tosi.log 2>&1
compare_rootfs "/tmp/tosi/packages/library-ruby/latest/ROOTFS"
