# Tosi

Pull and convert docker images to Milpa packages, using the power from the mother of the gods.

## Command line options

* repo: Docker repository to pull.
* reference: Image reference, default: latest.
* url: Docker registry URL to use, default: https://registry-1.docker.io/.
* username: Username for registry login. No (empty) username is used by default.
* password: Password for registry login. No (empty) password is used by default.
* workdir: Working directory, used for caching layers and packages. Default: /tmp/tosi.
* out: Path for Milpa package file that will be created. Default is constructed from the name of the repo and reference. For example, library/alpine 3.6 will be saved at /tmp/tosi/packages/library-alpine/3.6/library-alpine-pkg.tar.gz.
