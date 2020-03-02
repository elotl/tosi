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
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/distribution"
	manifestV1 "github.com/docker/distribution/manifest/schema1"
	"github.com/golang/glog"
	"github.com/ldx/docker-registry-client/registry"
	"github.com/opencontainers/go-digest"
)

const (
	ROOTFS_BASEDIR = "ROOTFS"
)

var (
	VERSION = "unknown"
)

// This is part of the config of docker images.
type HealthConfig struct {
	Test        []string      `json:",omitempty"`
	Interval    time.Duration `json:",omitempty"`
	Timeout     time.Duration `json:",omitempty"`
	StartPeriod time.Duration `json:",omitempty"`
	Retries     int           `json:",omitempty"`
}

// This is the main config struct for docker images.
type Config struct {
	Hostname        string
	Domainname      string
	User            string
	AttachStdin     bool
	AttachStdout    bool
	AttachStderr    bool
	ExposedPorts    map[string]struct{} `json:",omitempty"`
	Tty             bool
	OpenStdin       bool
	StdinOnce       bool
	Env             []string
	Cmd             []string
	Healthcheck     *HealthConfig `json:",omitempty"`
	ArgsEscaped     bool          `json:",omitempty"`
	Image           string
	Volumes         map[string]struct{}
	WorkingDir      string
	Entrypoint      []string
	NetworkDisabled bool   `json:",omitempty"`
	MacAddress      string `json:",omitempty"`
	OnBuild         []string
	Labels          map[string]string
	StopSignal      string   `json:",omitempty"`
	StopTimeout     *int     `json:",omitempty"`
	Shell           []string `json:",omitempty"`
}

type ImageConfig struct {
	Config Config `json:"config"`
}

func main() {
	version := flag.Bool("version", false, "Print current version")
	image := flag.String("image", "", "Docker repository to pull")
	url := flag.String("url", "https://registry-1.docker.io/", "Docker registry URL to use")
	username := flag.String("username", "", "Username for registry login")
	password := flag.String("password", "", "Password for registry login")
	workdir := flag.String("workdir", "/tmp/tosi", "Working directory, used for caching")
	out := flag.String("out", "", "Milpa package file to create")
	extractto := flag.String("extractto", "", "Only extract image to a directory, don't create package file")
	saveconfig := flag.String("saveconfig", "", "Save config of image to file as JSON")
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

	repo := *image
	dgest := ""
	reference := "latest"
	if strings.Contains(repo, "@") { // Exact hash for the image.
		parts := strings.Split(repo, "@")
		if len(parts) != 2 {
			glog.Fatalf("Invalid image specified %q", repo)
		}
		repo = parts[0]
		dgest = parts[1]
		d, err := digest.Parse(dgest)
		if err != nil {
			glog.Fatalf("Invalid hash specified in %q: %v", *image, err)
		}
		reference = d.String()
	}
	if strings.Contains(repo, ":") {
		parts := strings.Split(repo, ":")
		if len(parts) != 2 {
			glog.Fatalf("Invalid image specified %q", *image)
		}
		repo = parts[0]
		if dgest == "" {
			// Only use the tag if no digest is specified.
			reference = parts[1]
		}
	}

	layerdir := filepath.Join(*workdir, "layers")
	err := os.MkdirAll(layerdir, 0700)
	if err != nil {
		glog.Fatalf("Error creating %s: %v", layerdir, err)
	}

	configdir := filepath.Join(*workdir, "configs")
	err = os.MkdirAll(configdir, 0700)
	if err != nil {
		glog.Fatalf("Error creating %s: %v", configdir, err)
	}

	pkgbasedir := filepath.Join(*workdir, "packages")
	pkgdir, err := createPackageDir(pkgbasedir, *image, reference)
	if err != nil {
		glog.Fatalf("Error creating package directory for %s in %s: %v",
			*image, pkgbasedir, err)
	}

	rootfs := *extractto
	if rootfs == "" {
		// Create a temporary ROOTFS directory for extracting the image.
		rootfs, err = createRootfs(pkgdir)
		if err != nil {
			glog.Fatalf("Error creating ROOTFS for %s in %s", *image, pkgdir)
		}
		defer os.RemoveAll(rootfs)
	} else {
		// Extract into the specified directory, removing it first in case it
		// already exists.
		err = os.RemoveAll(rootfs)
		if err != nil {
			glog.Fatalf("Error removing %s for %s: %v", rootfs, *image, err)
		}
		err = os.MkdirAll(rootfs, 0700)
		if err != nil {
			glog.Fatalf("Error creating %s for %s: %v", rootfs, *image, err)
		}
	}

	reg, err := newRegistryClient(*url, *username, *password)
	if err != nil {
		glog.Fatalf("Error connecting to registry: %v", err)
	}

	var config string
	var refs []distribution.Descriptor
	manifest, err := reg.ManifestV2(repo, reference)
	if err != nil {
		glog.Warningf("Error retrieving v2 manifest: %v, trying v1", err)
	}
	if err != nil || manifest.Versioned.SchemaVersion == 1 {
		// Old, v1 manifest.
		manifestv1, err := reg.Manifest(repo, reference)
		if err != nil {
			glog.Fatalf("Error retrieving manifest: %v", err)
		}
		config, err = saveV1Config(configdir, repo, reference, manifestv1)
		if err != nil {
			glog.Fatalf("Error retrieving config: %v", err)
		}
		v1refs := manifestv1.References()
		for i := len(v1refs) - 1; i >= 0; i-- {
			// FSLayers might have duplicates.
			exists := false
			ref := v1refs[i]
			for _, r := range refs {
				if r.Digest == ref.Digest {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			refs = append(refs, ref)
		}
	} else if manifest.Versioned.SchemaVersion == 2 {
		config, err = saveBlob(reg, repo, configdir, manifest.Config)
		if err != nil {
			glog.Fatalf("Error retrieving config: %v", err)
		}
		refs = manifest.Layers
	} else {
		glog.Fatalf("Invalid manifest %+v", manifest)
	}

	files := make([]string, 0)
	for _, layer := range refs {
		name, err := saveBlob(reg, repo, layerdir, layer)
		if err != nil {
			glog.Fatalf("Error downloading layer %v: %v", layer, err)
		}
		files = append(files, name)
	}

	for _, f := range files {
		// All layers (which are actually .tar.gz files) are extracted into our
		// ROOTFS directory.
		wo, err := extractLayerToDir(f, rootfs)
		if err != nil {
			glog.Fatalf("Error processing layer %s: %v", f, err)
		}
		// Process whiteouts.
		err = processWhiteouts(rootfs, wo)
		if err != nil {
			glog.Fatalf("Error processing whiteouts in %s", rootfs)
		}
	}

	if *extractto == "" {
		// Create Milpa package.
		pkgpath, err := createPackage(pkgdir, repo, rootfs, *out)
		if err != nil {
			glog.Fatalf("Error creating package from %s: %v", rootfs, err)
		}
		glog.Infof("Package is available at %s", pkgpath)
	} else {
		glog.Infof("Image has been extracted into %s", rootfs)
	}

	dockerconf, err := getConfig(config)
	if err != nil {
		glog.Fatalf("Error reading config from %s: %v", config, err)
	}

	if *saveconfig != "" {
		err = saveAsJson(dockerconf, *saveconfig)
	}

	// Done!
	os.Exit(0)
}

// Creates a registry client with a shorter connection timeout, useful
// for inside AWS: https://github.com/elotl/milpa/issues/178
func newRegistryClient(registryUrl, username, password string) (*registry.Registry, error) {
	url := strings.TrimSuffix(registryUrl, "/")
	timeoutTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	transport := registry.WrapTransport(timeoutTransport, url, username, password)
	registry := &registry.Registry{
		URL: url,
		Client: &http.Client{
			Transport: transport,
		},
		Logf: registry.Log,
	}
	if err := registry.Ping(); err != nil {
		glog.Warningf("Pinging %s failed: %v", registryUrl, err)
	}
	return registry, nil
}

func saveV1Config(dir, repo, reference string, manifest *manifestV1.SignedManifest) (string, error) {
	if len(manifest.History) < 1 {
		return "", fmt.Errorf("V1 manifest with no history")
	}
	v1comp := manifest.History[0].V1Compatibility
	name := filepath.Join(
		dir,
		fmt.Sprintf("%s:%s", strings.Replace(repo, "/", "_", -1), reference))
	// Check if we already have the config downloaded.
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}
	os.Remove(name)
	err := ioutil.WriteFile(name, []byte(v1comp), 0644)
	if err != nil {
		glog.Errorf("Error saving config as %s: %v", name, err)
		return "", err
	}
	return name, nil
}

func saveAsJson(i interface{}, filename string) error {
	data, err := json.Marshal(i)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

func getConfig(configfile string) (*Config, error) {
	configdata, err := ioutil.ReadFile(configfile)
	if err != nil {
		return nil, err
	}
	ic := ImageConfig{}
	err = json.Unmarshal(configdata, &ic)
	if err != nil {
		return nil, err
	}
	glog.Infof("image config: %+v", ic)
	return &ic.Config, nil
}

func addPathToTar(tw *tar.Writer, path string, info os.FileInfo, rootfs string) error {
	if len(rootfs) > 0 && rootfs[len(rootfs)-1] == '/' {
		rootfs = rootfs[:len(rootfs)-1]
	}
	basedir := filepath.Dir(rootfs)
	rlen := len(rootfs)
	blen := len(basedir)
	name := path[blen:]
	if len(name) > 0 && name[0] == filepath.Separator {
		name = name[1:]
	}
	lname := ""
	isLink := false
	var err error
	if info.Mode()&os.ModeSymlink == os.ModeSymlink {
		isLink = true
		lname, err = os.Readlink(path)
		if err != nil {
			glog.Errorf("Error resolving symlink %s: %v", path, err)
			return err
		}
		if filepath.IsAbs(lname) && len(lname) > rlen && lname[:rlen] == rootfs {
			lname = lname[rlen:]
		}
	}
	header, err := tar.FileInfoHeader(info, lname)
	if err != nil {
		glog.Errorf("Creating tar header from %s: %v", path, err)
		return err
	}
	header.Name = name
	if err = tw.WriteHeader(header); err != nil {
		glog.Errorf("Writing tar header for %s: %v", path, err)
		return err
	}
	if isLink {
		glog.Infof("%s -> %s %v", name, lname, header.FileInfo().Mode())
	} else {
		glog.Infof("%s %v", name, header.FileInfo().Mode())
	}
	if info.IsDir() || isLink {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		glog.Errorf("Opening %s: %v", path, err)
		return err
	}
	defer file.Close()
	if _, err = io.Copy(tw, file); err != nil {
		glog.Errorf("Writing %s data into tarball: %v", path, err)
		return err
	}
	return nil
}

func createPackage(pkgdir, repo, rootfs, pkgpath string) (string, error) {
	if pkgpath == "" {
		name := strings.Replace(repo, "/", "-", -1)
		name = strings.Replace(name, ":", "-", -1) + "-pkg.tar.gz"
		pkgpath = filepath.Join(pkgdir, name)
	}
	tmpname := filepath.Join(filepath.Dir(pkgpath), "."+filepath.Base(pkgpath))
	file, err := os.Create(tmpname)
	if err != nil {
		glog.Errorf("Error creating temporary package file %s: %v", tmpname, err)
		return "", err
	}
	defer os.Remove(tmpname)
	defer file.Close()
	gw := gzip.NewWriter(file)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	err = filepath.Walk(rootfs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return addPathToTar(tw, path, info, rootfs)
	})
	if err != nil {
		glog.Errorf("Error adding rootfs contents to package %s: %v",
			pkgpath, err)
		return "", err
	}
	err = os.Rename(tmpname, pkgpath)
	if err != nil {
		glog.Errorf("Error renaming package file %s -> %s: %v",
			tmpname, pkgpath, err)
		return "", err
	}
	return pkgpath, nil
}

func createPackageDir(basedir, repo, ref string) (string, error) {
	pkgname := strings.Replace(repo, "/", "-", -1)
	pkgname = strings.Replace(pkgname, ":", "-", -1)
	path := filepath.Join(basedir, pkgname, ref)
	err := os.MkdirAll(path, 0700)
	if err != nil {
		glog.Errorf("Creating %s: %v", path, err)
		return "", err
	}
	return path, nil
}

func createRootfs(pkgdir string) (string, error) {
	rootfspath := filepath.Join(pkgdir, ROOTFS_BASEDIR)
	err := os.RemoveAll(rootfspath)
	if err != nil {
		glog.Errorf("Removing %s: %v", rootfspath, err)
		return "", err
	}
	err = os.MkdirAll(rootfspath, 0700)
	if err != nil {
		glog.Errorf("Creating %s: %v", rootfspath, err)
		return "", err
	}
	return rootfspath, nil
}

type Link struct {
	dst      string
	src      string
	linktype byte
	mode     os.FileMode
	uid      int
	gid      int
}

func extractLayerToDir(filename, destdir string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	var links []Link
	var whiteouts []string

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		name := filepath.Join(destdir, header.Name)
		basename := filepath.Base(name)
		if basename == ".wh..wh..opq" {
			// Whiteout opaque directory. We don't need to do anything with
			// it (only meaningful for layered filesystems).
			continue
		}
		if len(basename) > 4 && basename[:4] == ".wh." {
			// Reconstruct path without the .wh. prefix in the filename.
			path := filepath.Join(filepath.Dir(name), basename[4:])
			glog.Infof("Found whiteout %s for %s", name, path)
			whiteouts = append(whiteouts, path)
			continue
		}

		mode := header.FileInfo().Mode()

		glog.Infof("%s %v %v", name, header.Typeflag, mode)

		dir := filepath.Dir(name)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
		switch header.Typeflag {
		case tar.TypeDir: // directory
			err := os.Mkdir(name, mode)
			if err != nil && !os.IsExist(err) {
				glog.Errorf("Extracting %s mkdir: %v", name, err)
				return nil, err
			}
			err = os.Chmod(name, mode)
			if err != nil {
				glog.Errorf("Extracting %s chmod: %v", name, err)
				return nil, err
			}
		case tar.TypeReg: // regular file
			data := make([]byte, header.Size)
			read_so_far := int64(0)
			for read_so_far < header.Size {
				n, err := tr.Read(data[read_so_far:])
				if err != nil && err != io.EOF {
					glog.Errorf("Extracting %s: %v", name, err)
					return nil, err
				}
				read_so_far += int64(n)
			}
			if read_so_far != header.Size {
				glog.Errorf("Extracting %s: read %d bytes, but size is %d bytes",
					name, read_so_far, header.Size)
			}
			// Ensure nothing exists at this path first.
			os.Remove(name)
			err = ioutil.WriteFile(name, data, mode)
			if err != nil {
				glog.Errorf("Extracting %s writing content: %v", name, err)
				return nil, err
			}
			err = os.Chmod(name, mode)
			if err != nil {
				glog.Errorf("Extracting %s chmod: %v", name, err)
				return nil, err
			}
		case tar.TypeLink, tar.TypeSymlink:
			linkname := header.Linkname
			// Hard links will need a valid absolute path. Update them,
			// relative to destdir.
			if header.Typeflag == tar.TypeLink {
				linkname = filepath.Join(destdir, linkname)
			}
			// Links might point to files or directories that have not been
			// extracted from the tarball yet. Create them after going through
			// all entries in the tarball.
			links = append(links, Link{linkname, name, header.Typeflag, mode, header.Uid, header.Gid})
			continue
		default:
			glog.Warningf(
				"Ignoring unknown type while extracting %s (layer %s): %d",
				name, filename, header.Typeflag)
			continue
		}
		err = os.Chown(name, header.Uid, header.Gid)
		if err != nil {
			//glog.Infof("Warning: chown %s type %d uid %d gid %d: %v",
			//	name, header.Typeflag, header.Uid, header.Gid, err)
		}
	}

	for _, link := range links {
		os.Remove(link.src) // Remove link in case it exists.
		if link.linktype == tar.TypeSymlink {
			//glog.Infof("s %s -> %s", link.src, link.dst)
			err = os.Symlink(link.dst, link.src)
			if err != nil {
				glog.Errorf("Creating symlink %s -> %s: %v",
					link.src, link.dst, err)
				return nil, err
			}
			err = os.Lchown(link.src, link.uid, link.gid)
			if err != nil {
				//glog.Warningf("Warning: chown symlink %s uid %d gid %d: %v",
				//	link.src, link.uid, link.gid, err)
			}
		}
		if link.linktype == tar.TypeLink {
			//glog.Infof("h %s", link.src)
			err = os.Link(link.dst, link.src)
			if err != nil {
				glog.Errorf("Creating hardlink %s -> %s: %v",
					link.src, link.dst, err)
				return nil, err
			}
			err = os.Chmod(link.src, link.mode)
			if err != nil {
				glog.Errorf("Chmod hardlink %s %d: %v",
					link.src, link.mode, err)
				return nil, err
			}
			err = os.Chown(link.src, link.uid, link.gid)
			if err != nil {
				//glog.Warningf("Warning: chown hardlink %s uid %d gid %d: %v",
				//	link.src, link.uid, link.gid, err)
			}
		}
	}

	return whiteouts, nil
}

func processWhiteouts(rootfs string, whiteouts []string) error {
	for _, wo := range whiteouts {
		fi, err := os.Lstat(wo)
		if err != nil && os.IsNotExist(err) {
			glog.Warningf("Whiteout %s does not exist, ignoring", wo)
			continue
		}
		mode := fi.Mode()
		if mode.IsDir() {
			glog.Infof("Removing whiteout directory %s", wo)
			err = os.RemoveAll(wo)
		} else {
			glog.Infof("Removing whiteout file %s", wo)
			err = os.Remove(wo)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func isLayerValid(name string, dgst digest.Digest) bool {
	verifier := dgst.Verifier()
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = io.Copy(verifier, f)
	if err != nil {
		return false
	}
	return verifier.Verified()
}

func saveBlob(reg *registry.Registry, repo, dir string, desc distribution.Descriptor) (string, error) {
	tmpname := filepath.Join(dir, "."+desc.Digest.String())
	defer os.Remove(tmpname)
	name := filepath.Join(dir, desc.Digest.String())
	// Check if we already have the blob downloaded.
	if _, err := os.Stat(name); err == nil {
		// Blob file already exists. Check its hash.
		if isLayerValid(name, desc.Digest) {
			glog.Infof("Repo %s blob %s already exists and valid", repo, name)
			return name, nil
		}
	}
	os.Remove(name)
	glog.Infof("Saving repo %s blob %s", repo, name)
	reader, err := reg.DownloadLayer(repo, desc.Digest)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	f, err := os.Create(tmpname)
	if err != nil {
		return "", err
	}
	defer f.Close()
	verifier := desc.Digest.Verifier()
	writer := io.MultiWriter(f, verifier)
	n, err := io.Copy(writer, reader)
	if err != nil {
		return "", err
	}
	f.Close()
	if n < desc.Size {
		return "", fmt.Errorf("Error saving %s: wrote only %d/%d bytes",
			name, n, desc.Size)
	}
	if !verifier.Verified() {
		return "", fmt.Errorf("Error saving %s: verifier failed", name)
	}
	err = os.Rename(tmpname, name)
	if err != nil {
		return "", err
	}
	return name, nil
}
