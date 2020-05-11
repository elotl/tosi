# Tosi

Pull and extract container images. Tosi can pull images from image registries, and extract them to a flat directory, or create an [overlayfs](https://www.kernel.org/doc/Documentation/filesystems/overlayfs.txt) mount point with the layers as lowerdirs.

## Build

You can simply use:

    $ make

## Usage

To pull an image, and unpack all layers into `/tmp/nginx-rootfs`:

    tosi -image library/nginx -extractto /tmp/nginx-rootfs

Create an overlayfs mount in `/run/rootfs` from `quay.io/quay/redis`:

    tosi -url quay.io -image quay/redis -mount /run/rootfs

Don't forget to unmount the overlayfs when done:

    umount /run/rootfs

Pull an image with a specific tag, without unpacking it, and save its config for inspection:

    tosi -image bitnami/tomcat:8.5.31 -saveconfig /tmp/tomcat.config

Pull a large image using 8 parallel threads for fetching layers:

    tosi -url quay.io -image bitriseio/android-ndk -parallel-downloads 8

To use a cache directory at `/mnt/image-cache`:

    tosi -workdir /mnt/image-cache -image library/alpine

Tosi caches already downloaded layers, and can reuse layers for creating overlayfs mounts.

Check the speedup from caching layers:

    time tosi -workdir /mnt/image-cache -image library/ubuntu
    [...]
    real    0m10,693s
    user    0m1,416s
    sys     0m0,402s
    # Second time it should be a lot faster.
    time tosi -workdir /mnt/image-cache -image library/ubuntu
    [...]
    real    0m2,873s
    user    0m0,160s
    sys     0m0,055s

## Command line options

 * -alsologtostderr
        log to standard error as well as files
 * -extractto string
        Extract and combine all layers of an image directly into this directory. Mutually exclusive with -mount <dir>.
 * -image string
        Image repository to pull. Usual conventions can be used; e.g. library/alpine:3.6 to specify the repository library/alpine and the tag 3.6. See also -url.
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
        Registry base URL to use. For example, to pull quay.io/prometheus/node-exporter you need to specify "-url quay.io -image prometheus/node-exporter". See also -image. (default "https://registry-1.docker.io/")
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
