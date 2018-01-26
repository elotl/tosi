package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/distribution"
	"github.com/golang/glog"
	"github.com/heroku/docker-registry-client/registry"
	"github.com/opencontainers/go-digest"
)

const (
	TAR_BASEDIR = "ROOTFS"
)

func main() {
	repo := flag.String("repo", "", "Docker repository to pull")
	reference := flag.String("reference", "latest", "Image reference")
	url := flag.String("url", "https://registry-1.docker.io/", "Docker registry URL to use")
	username := flag.String("username", "", "Username for registry login")
	password := flag.String("password", "", "Password for registry login")
	workdir := flag.String("workdir", "/tmp/tosi", "Working directory, used for caching")
	out := flag.String("out", "", "Milpa package file to create")
	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	if *repo == "" {
		glog.Fatalf("Please specify repo")
	}

	layerdir := filepath.Join(*workdir, "layers")
	err := os.MkdirAll(layerdir, 0700)
	if err != nil {
		glog.Fatalf("Error retrieving manifest: %v", err)
	}

	pkgdir := filepath.Join(*workdir, "packages")
	err = os.MkdirAll(pkgdir, 0700)
	if err != nil {
		glog.Fatalf("Error retrieving manifest: %v", err)
	}

	reg, err := registry.New(*url, *username, *password)
	if err != nil {
		glog.Fatalf("Error connecting to registry: %v", err)
	}

	manifest, err := reg.ManifestV2(*repo, *reference)
	if err != nil {
		glog.Fatalf("Error retrieving manifest: %v", err)
	}

	files := make([]string, 0)
	for _, layer := range manifest.Layers {
		name, err := saveLayer(reg, *repo, layerdir, layer)
		if err != nil {
			glog.Fatalf("Error downloading layer %v: %v", layer, err)
		}
		files = append(files, name)
	}

	rootfs, err := createRootfs(pkgdir, *repo)
	if err != nil {
		glog.Fatalf("Error creating ROOTFS for %s in %s", *repo, pkgdir)
	}
	defer os.RemoveAll(rootfs)

	var whiteouts []string
	for _, f := range files {
		// All layers (which are actually .tar.gz files) are extracted into a
		// ROOTFS directory.
		wo, err := extractLayerToDir(f, rootfs)
		if err != nil {
			glog.Fatalf("Error processing layer %s: %v", f, err)
		}
		whiteouts = append(whiteouts, wo...)
	}

	// Process whiteouts.
	err = processWhiteouts(rootfs, whiteouts)
	if err != nil {
		glog.Fatalf("Error processing whiteouts in %s", rootfs)
	}

	pkgpath, err := createPackage(pkgdir, *repo, rootfs, *out)
	if err != nil {
		glog.Fatalf("Error creating package from %s: %v", rootfs, err)
	}
	glog.Infof("Package is available at %s", pkgpath)

	// Done!
	os.Exit(0)
}

func addPathToTar(tw *tar.Writer, path string, info os.FileInfo, rootfs string) error {
	if len(rootfs) > 0 && rootfs[len(rootfs)-1] == '/' {
		rootfs = rootfs[:len(rootfs)-1]
	}
	basedir := filepath.Dir(rootfs)
	rlen := len(rootfs)
	blen := len(basedir)
	file, err := os.Open(path)
	if err != nil {
		glog.Errorf("Opening %s: %v", path, err)
		return err
	}
	defer file.Close()
	name := path[blen:]
	if len(name) > 0 && name[0] == filepath.Separator {
		name = name[1:]
	}
	lname := ""
	isLink := false
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
		glog.Infof("%s -> %s", name, lname)
	} else {
		glog.Infof("%s", name)
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
	if info.IsDir() || isLink {
		return nil
	}
	if _, err = io.Copy(tw, file); err != nil {
		glog.Errorf("Writing %s data into tarball: %v", path, err)
		return err
	}
	return nil
}

func createPackage(pkgdir, repo, rootfs, pkgname string) (string, error) {
	if pkgname == "" {
		pkgname = strings.Replace(repo, "/", "-", -1)
		pkgname = strings.Replace(pkgname, ":", "-", -1) + "-pkg.tar.gz"
	}
	tmpname := filepath.Join(filepath.Dir(pkgname), "."+filepath.Base(pkgname))
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
			pkgname, err)
		return "", err
	}
	err = os.Rename(tmpname, pkgname)
	if err != nil {
		glog.Errorf("Error renaming package file %s -> %s: %v",
			tmpname, pkgname, err)
		return "", err
	}
	return pkgname, nil
}

func createRootfs(pkgdir, repo string) (string, error) {
	pkgname := strings.Replace(repo, "/", "-", -1)
	pkgname = strings.Replace(pkgname, ":", "-", -1)
	rootfspath := filepath.Join(pkgdir, pkgname, "ROOTFS")
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

		switch header.Typeflag {
		case tar.TypeDir: // directory
			os.Mkdir(name, os.FileMode(header.Mode))
		case tar.TypeReg: // regular file
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
			ioutil.WriteFile(name, data, os.FileMode(header.Mode))
		case tar.TypeLink, tar.TypeSymlink:
			linkname := header.Linkname
			// Preserve relative symlinks (pointing to a file/directory in the
			// same directory or starting with "../").
			if header.Typeflag == tar.TypeLink || filepath.IsAbs(linkname) {
				linkname = filepath.Join(destdir, linkname)
			}
			// Links might point to files or directories that have not been
			// extracted from the tarball yet. Create them after going through
			// all entries in the tarball.
			links = append(links, Link{linkname, name, header.Typeflag, os.FileMode(header.Mode), header.Uid, header.Gid})
			continue
		default:
			glog.Warningf("Unknown type while extracting layer %s: %d",
				filename, header.Typeflag)
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

func saveLayer(reg *registry.Registry, repo, layerdir string, layer distribution.Descriptor) (string, error) {
	tmpname := filepath.Join(layerdir, "."+layer.Digest.String())
	defer os.Remove(tmpname)
	name := filepath.Join(layerdir, layer.Digest.String())
	// Check if we already have the layer downloaded.
	if _, err := os.Stat(name); err == nil {
		// Layer file already exists. Check its hash.
		if isLayerValid(name, layer.Digest) {
			glog.Infof("Repo %s layer %s already exists and valid", repo, name)
			return name, nil
		}
	}
	os.Remove(name)
	glog.Infof("Saving repo %s layer %s", repo, name)
	reader, err := reg.DownloadLayer(repo, layer.Digest)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	f, err := os.Create(tmpname)
	if err != nil {
		return "", err
	}
	defer f.Close()
	verifier := layer.Digest.Verifier()
	writer := io.MultiWriter(f, verifier)
	n, err := io.Copy(writer, reader)
	if err != nil {
		return "", err
	}
	f.Close()
	if n != layer.Size {
		return "", fmt.Errorf("Error saving %s: wrote only %d/%d bytes",
			name, n, layer.Size)
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
