# Tosi

Pull and extract container images. Tosi can pull images from image registries, and extract them to a flat directory, or create an [overlayfs](https://www.kernel.org/doc/Documentation/filesystems/overlayfs.txt) mount point with the layers as lowerdirs.

## Build

You can simply use:

    $ make

## Usage

To pull an image, and unpack all layers into `/tmp/nginx-rootfs`:

    tosi -image nginx -extractto /tmp/nginx-rootfs

Create an overlayfs mount in `/run/rootfs` from `quay.io/quay/redis`:

    tosi -image quay.io/quay/redis -mount /run/rootfs

Don't forget to unmount the overlayfs when done:

    umount /run/rootfs

Pull an image with a specific tag, without unpacking it, and save its config for inspection:

    tosi -image bitnami/tomcat:8.5.31 -saveconfig /tmp/tomcat.config

Pull a large image using 8 parallel threads for fetching layers:

    tosi -image quay.io/bitriseio/android-ndk -parallel-downloads 8

To use a cache directory at `/mnt/image-cache`:

    tosi -workdir /mnt/image-cache -image alpine

Tosi caches already downloaded layers, and can reuse layers for creating overlayfs mounts.

Check the speedup from caching layers:

    time tosi -workdir /mnt/image-cache -image ubuntu
    [...]
    real    0m10,693s
    user    0m1,416s
    sys     0m0,402s
    # Second time it should be a lot faster.
    time tosi -workdir /mnt/image-cache -image ubuntu
    [...]
    real    0m2,873s
    user    0m0,160s
    sys     0m0,055s

## How it works

Docker (and other modern container runtime, like containerd or CRI-O) containers are images with a writeable layer on top of one or more read-only layers. The read-only layers are created when an image is built, for example via `docker build`. The layers when saved are tarballs of all the files and directories created in a particular step of the image build. Images are stored on registry servers.

Tosi can fetch these layers and create a snapshot of all of them extracted or mounted via overlayfs into a directory, along with metadata, such as the image "config" (which provides overrideable defaults such as environment variables, entrypoint, etc) and the manifest describing the image.

If you look at the directories created by tosi:

    $ ls -1 /tmp/tosi
    configs/
    layers/
    manifests/
    overlays/

Here, "configs" contains the image configs, "manifests" the image manifests. The directory "layers" contains the layer tarballs, and finally, "overlays" contains directories with extracted layers.

For example, the image `alpine:3.6` has only one layer:

    {
       "schemaVersion": 2,
       "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
       "config": {
          "mediaType": "application/vnd.docker.container.image.v1+json",
          "size": 1512,
          "digest": "sha256:43773d1dba76c4d537b494a8454558a41729b92aa2ad0feb23521c3e58cd0440"
       },
       "layers": [
          {
             "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
             "size": 2017774,
             "digest": "sha256:5a3ea8efae5d0abb93d2a04be0a4870087042b8ecab8001f613cdc2a9440616a"
          }
       ]
    }

It is a tar.gz file:

    $ file /tmp/tosi/layers/5a3ea8efae5d0abb93d2a04be0a4870087042b8ecab8001f613cdc2a9440616a
    /tmp/tosi/layers/5a3ea8efae5d0abb93d2a04be0a4870087042b8ecab8001f613cdc2a9440616a: gzip compressed data, original size 4282880

You can check out the contents of the layer:

    $ ls -1 /tmp/tosi/overlays/5a3ea8efae5d0abb93d2a04be0a4870087042b8ecab8001f613cdc2a9440616a
    bin/
    dev/
    etc/
    home/
    lib/
    media/
    mnt/
    proc/
    root/
    run/
    sbin/
    srv/
    sys/
    tmp/
    usr/
    var/

Tosi also creates a short link, since overlayfs has a limit on the length of all the layer names when creating a mount. You can check the short link:

    $ cat /tmp/tosi/overlays/5a3ea8efae5d0abb93d2a04be0a4870087042b8ecab8001f613cdc2a9440616a.link
    1s8vir6felvja
    $ ls -l /tmp/tosi/overlays/1s8vir6felvja
    lrwxrwxrwx 1 root root 64 máj   19 10:04 /tmp/tosi/overlays/1s8vir6felvja -> 5a3ea8efae5d0abb93d2a04be0a4870087042b8ecab8001f613cdc2a9440616a/
    $ tosi -image alpine:3.6 -mount /tmp/alpine-rootfs
    [...]
    $ mount | grep alpine
    overlay on /tmp/alpine-rootfs type overlay (rw,relatime,lowerdir=1s8vir6felvja,upperdir=/tmp/alpine-rootfs.upper,workdir=/tmp/alpine-rootfs.work)

## Command line options

* -alsologtostderr
   	log to standard error as well as files
* -extractto string
   	Extract and combine all layers of an image directly into this directory. Mutually exclusive with -mount <dir>.
* -image string
   	Image repository to pull. Usual conventions can be used; e.g. library/alpine:3.6 to specify the repository library/alpine and the tag 3.6.
* -log_backtrace_at value
   	when logging hits line file:N, emit a stack trace
* -log_dir string
   	If non-empty, write log files in this directory
* -logtostderr
   	log to standard error instead of files
* -mount string
   	Create an overlayfs mount in this directory, which creates a writable mount that is a combined view of all the image layers. Mutually exclusive with -extractto <dir>. The directory will be created if it does not exist.
* -overlaydir string
   	Working directory for extracting layers. By default, it will be <workdir>/overlays.
* -parallel-downloads int
   	Number of parallel downloads when pulling images. (default 4)
* -password string
   	Password for registry login. Leave it empty if no login is required for pulling the image.
* -saveconfig string
   	Save config from image to this file as JSON.
* -stderrthreshold value
   	logs at or above this threshold go to stderr
* -url string
   	DEPRECATED. Use -image instead with the registry server as the first part, e.g. quay.io/myuser/myimage.
* -username string
   	Username for registry login. Leave it empty if no login is required for pulling the image.
* -v value
   	log level for V logs
* -validate-cache
   	Enable to validate already downloaded layers in cache via verifying their checksum.
* -version
   	Print current version and exit.
* -vmodule value
   	comma-separated list of pattern=N settings for file-filtered logging
* -workdir string
   	Working directory for downloading layers and other metadata. This directory will be effectively used as a cache of images and layers. Do not modify any file inside it. (default "/tmp/tosi")
