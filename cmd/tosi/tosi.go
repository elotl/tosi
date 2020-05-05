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
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/elotl/tosi/pkg/registryclient"
	"github.com/elotl/tosi/pkg/store"
	"github.com/golang/glog"
)

const (
	ROOTFS_BASEDIR = "ROOTFS"
)

var (
	VERSION = "unknown"
)

type ImageConfig struct {
	Config container.Config `json:"config"`
}

func main() {
	version := flag.Bool("version", false, "Print current version")
	image := flag.String("image", "", "Image repository to pull")
	url := flag.String("url", "https://registry-1.docker.io/", "Registry URL to use")
	username := flag.String("username", "", "Username for registry login")
	password := flag.String("password", "", "Password for registry login")
	workdir := flag.String("workdir", "/tmp/tosi", "Working directory for downloading layers and other metadata.")
	overlaydir := flag.String("overlaydir", "", "Working directory for extracting layers. By default, the it will be <workdir>/overlays.")
	extractto := flag.String("extractto", "", "Extract all layers of an image directly into this directory. Mutually exclusive with -mount <dir>.")
	mount := flag.String("mount", "", "Create an overlayfs mount in this directory. Mutually exclusive with -extractto <dir>. The directory will be created if it does not exist.")
	saveconfig := flag.String("saveconfig", "", "Save config from image to file as JSON")
	parallelism := flag.Int("parallel-downloads", 4, "Number of parallel downloads when pulling images")
	validate := flag.Bool("validate-cache", false, "Enable to validate already downloaded layers in cache via verifying their checksum")
	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	glog.Infof("Tosi version %s", VERSION)

	if *version {
		os.Exit(0)
	}

	if *image == "" {
		glog.Fatalf("Please specify image to pull")
	}

	if strings.HasPrefix(*image, "k8s.gcr.io/") {
		// k8s.gcr.io is an alias used by GCR.
		*image = "google_containers/" + strings.TrimPrefix(*image, "k8s.gcr.io/")
		*url = "https://gcr.io/"
	}

	rootfs := *extractto
	if rootfs != "" {
		if *mount != "" {
			glog.Fatalf("-extractto and -mount are mutually exclusive")
		}
		// Extract into the specified directory, removing it first in case it
		// already exists.
		err := os.RemoveAll(rootfs)
		if err != nil {
			glog.Fatalf("removing %s for %s: %v", rootfs, *image, err)
		}
		err = os.MkdirAll(rootfs, 0700)
		if err != nil {
			glog.Fatalf("creating %s for %s: %v", rootfs, *image, err)
		}
	}

	reg, err := registryclient.NewRegistryClient(
		*url, *username, *password, *validate)
	if err != nil {
		glog.Fatalf("connecting to registry %s: %v", *url, err)
	}

	store, err := store.NewStore(*workdir, *overlaydir, *parallelism, reg)
	if err != nil {
		glog.Fatalf("creating image store in %s: %v", *workdir, err)
	}
	_, err = store.Pull(*image)
	if err != nil {
		glog.Fatalf("pulling image %s: %v", *image, err)
	}

	if rootfs != "" {
		err = store.Unpack(*image, rootfs)
		if err != nil {
			glog.Fatalf("unpacking %s into %s: %v", *image, rootfs, err)
		}
	}

	if *mount != "" {
		err = store.Mount(*image, *mount)
		if err != nil {
			glog.Fatalf("mounting %s into /tmp/image: %v", *image, err)
		}
	}

	if *saveconfig != "" {
		err = store.SaveConfig(*image, *saveconfig)
		if err != nil {
			glog.Fatalf(
				"saving config for %s to %s: %v", *image, *saveconfig, err)
		}
	}

	// Done!
	os.Exit(0)
}
