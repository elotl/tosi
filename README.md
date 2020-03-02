# Tosi

Pull and extract docker images.

## Command line options

* `version`: Print out version and exit.
* `image <image>`: Image to pull. Usual conventions can be used; e.g. library/alpine:3.6 to specify the repository `library/alpine` and the tag `3.6`.
* `url <url>`: Registry base URL to use, default: https://registry-1.docker.io/.
* `username <user>`: Username for registry login. No (empty) username is used by default.
* `password <password>`: Password for registry login. No (empty) password is used by default.
* `workdir <directory>`: Working directory, used for downloading and caching layers and packages. Default: `/tmp/tosi`.
* `out <path>`: Path for flat tarball package file that will be created after processing layers. Default is constructed from the name of the repo and reference. For example, library/alpine 3.6 will be saved at `/tmp/tosi/packages/library-alpine/3.6/library-alpine-pkg.tar.gz`.
* `extractto <directory>`: Only extract image to this directory, don't create tarball. If not specified, the rootfs will be extracted to `<workdir>/packages/<repository>/<tag>/ROOTFS`.
* `saveconfig <path>`: Save config from image as JSON to this path.
