#!/bin/bash

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..
cd $ROOT_DIR
go test ./...

compare_rootfs() {
    rootfs="$1"
    pushd "$rootfs"
    find . -exec ls -d -l {} \; | awk '{ s = ""; for (i = 9; i <= NF; i++) s = s $i " "; print s }' | sed "s#$rootfs##g" | sort > /tmp/files1.txt
    cd ..
    mkdir -p check
    cd check
    tar xvzf ../*.tar.gz
    cd ROOTFS
    find . -exec ls -d -l {} \; | awk '{ s = ""; for (i = 9; i <= NF; i++) s = s $i " "; print s }' | sort > /tmp/files2.txt
    cmp /tmp/files1.txt /tmp/files2.txt
    popd
}

./tosi -repo library/alpine -reference 3.6
compare_rootfs "/tmp/tosi/packages/library-alpine/3.6/ROOTFS"

./tosi -repo library/ruby -reference latest
compare_rootfs "/tmp/tosi/packages/library-ruby/latest/ROOTFS"
