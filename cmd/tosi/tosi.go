/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/elotl/tosi/pkg/registryclient"
	"github.com/elotl/tosi/pkg/store"
	"github.com/elotl/tosi/pkg/util"
	"github.com/golang/glog"
)

const (
	ROOTFS_BASEDIR = "ROOTFS"
)

var (
	Version = "unknown"
)

type ImageConfig struct {
	Config container.Config `json:"config"`
}

func main() {
	version := flag.Bool("version", false, "Print current version and exit.")
	image := flag.String("image", "", "Image repository to pull. Usual conventions can be used; e.g. library/alpine:3.6 to specify the repository library/alpine and the tag 3.6.")
	url := flag.String("url", "", "DEPRECATED. Use -image instead with the registry server as the first part, e.g. quay.io/myuser/myimage.")
	username := flag.String("username", "", "Username for registry login. Leave it empty if no login is required for pulling the image.")
	password := flag.String("password", "", "Password for registry login. Leave it empty if no login is required for pulling the image.")
	workdir := flag.String("workdir", "/tmp/tosi", "Working directory for downloading layers and other metadata. This directory will be effectively used as a cache of images and layers. Do not modify any file inside it.")
	overlaydir := flag.String("overlaydir", "", "Working directory for extracting layers. By default, it will be <workdir>/overlays.")
	extractto := flag.String("extractto", "", "Extract and combine all layers of an image directly into this directory. Mutually exclusive with -mount <dir>.")
	mount := flag.String("mount", "", "Create an overlayfs mount in this directory, which creates a writable mount that is a combined view of all the image layers. Mutually exclusive with -extractto <dir>. The directory will be created if it does not exist.")
	saveconfig := flag.String("saveconfig", "", "Save config from image to this file as JSON.")
	parallelism := flag.Int("parallel-downloads", 4, "Number of parallel downloads when pulling images.")
	validate := flag.Bool("validate-cache", false, "Enable to validate already downloaded layers in cache via verifying their checksum.")
	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	progname := "tosi"
	if len(os.Args) > 0 {
		progname = filepath.Base(os.Args[0])
	}

	if *version {
		fmt.Printf("%s version %s\n", progname, Version)
		os.Exit(0)
	}

	glog.Infof("%s version: %s", progname, Version)

	if *image == "" {
		glog.Fatalf("Please specify image to pull")
	}

	registry := *url
	img := *image
	if registry == "" {
		registry, img = util.ParseFullImage(*image)
	}
	glog.Infof("pulling image %q from registry %q", img, registry)

	rootfs := *extractto
	if rootfs != "" {
		if *mount != "" {
			glog.Fatalf("-extractto and -mount are mutually exclusive")
		}
		// If rootfs already exists, it needs to be empty.
		if util.PathExists(rootfs) && !util.IsEmptyDir(rootfs) {
			glog.Fatalf("%s is not empty or accessible", rootfs)
		}
	}

	reg, err := registryclient.NewRegistryClient(
		registry, *username, *password, *validate)
	if err != nil {
		glog.Fatalf("connecting to registry %s: %v", registry, err)
	}

	store, err := store.NewStore(*workdir, *overlaydir, *parallelism, reg)
	if err != nil {
		glog.Fatalf("creating image store in %s: %v", *workdir, err)
	}
	_, err = store.Pull(img)
	if err != nil {
		glog.Fatalf("pulling image %s: %v", img, err)
	}

	if rootfs != "" {
		err = store.Unpack(img, rootfs)
		if err != nil {
			glog.Fatalf("unpacking %s into %s: %v", img, rootfs, err)
		}
	}

	if *mount != "" {
		err = store.Mount(img, *mount)
		if err != nil {
			glog.Fatalf("mounting %s into %s: %v", img, *mount, err)
		}
	}

	if *saveconfig != "" {
		err = store.SaveConfig(img, *saveconfig)
		if err != nil {
			glog.Fatalf("saving config for %s to %s: %v", img, *saveconfig, err)
		}
	}

	// Done!
	glog.Infof("Success!")
	os.Exit(0)
}
