package store

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
	"github.com/elotl/tosi/pkg/manifest"
	"github.com/elotl/tosi/pkg/registryclient"
	"github.com/elotl/tosi/pkg/util"
	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
)

const (
	maxRetries = 10
)

type Store struct {
	BaseDir           string
	layerDir          string
	configDir         string
	manifestDir       string
	overlayDir        string
	parallelDownloads int
	reg               *registryclient.RegistryClient
}

// NewStore creates a new image store, with basedir as the base directory for
// storing layers and metadata, and overlaydir as the directory layers will be
// unpacked into. The filesystem backing overlaydir needs to support special
// files like device files and sockets. The parameter parallelism can be used
// to parallelize layer downloads and unpacking. The parameter reg is a
// RegistryClient.
func NewStore(basedir string, overlaydir string, parallelism int, reg *registryclient.RegistryClient) (*Store, error) {
	layerdir := filepath.Join(basedir, "layers")
	configdir := filepath.Join(basedir, "configs")
	manifestdir := filepath.Join(basedir, "manifests")
	if overlaydir == "" {
		overlaydir = filepath.Join(basedir, "overlays")
	}
	for _, dir := range []string{layerdir, configdir, manifestdir, overlaydir} {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return nil, fmt.Errorf("creating %s: %v", dir, err)
		}
	}
	if parallelism < 0 {
		parallelism = 1
	}
	return &Store{
		BaseDir:           basedir,
		layerDir:          layerdir,
		configDir:         configdir,
		manifestDir:       manifestdir,
		overlayDir:        overlaydir,
		parallelDownloads: parallelism,
		reg:               reg,
	}, nil
}

func (s *Store) doPull(repo string, wg *sync.WaitGroup, layers chan distribution.Descriptor, results chan error) {
	wg.Add(1)
	defer wg.Done()
	for layer := range layers {
		glog.V(2).Infof("pulling %s layer %+v", repo, layer.Digest.String())
		_, err := s.reg.SaveBlob(repo, s.layerDir, layer)
		if err != nil {
			results <- fmt.Errorf("downloading layer %v: %v", layer, err)
			continue
		}
		glog.V(2).Infof("unpacking %s layer %+v", repo, layer.Digest.String())
		dgest := layer.Digest.Encoded()
		into := filepath.Join(s.overlayDir, dgest)
		if _, err = os.Stat(into); err != nil {
			err = s.unpackLayer(dgest, into, true)
		}
		if err == nil {
			err = s.createShortLink(into)
		}
		if err != nil {
			results <- fmt.Errorf("unpacking layer %v: %v", layer, err)
			continue
		}
		results <- nil
	}
}

func (s *Store) pullLayers(repo string, mfest *manifest.Manifest) error {
	wg := &sync.WaitGroup{}
	layers := mfest.Layers()
	layerCh := make(chan distribution.Descriptor, len(layers))
	resultCh := make(chan error, len(layers))
	parallelism := s.parallelDownloads
	if parallelism <= 0 {
		parallelism = len(layers)
	}
	glog.V(2).Infof("starting %d workers for pulling %s", parallelism, repo)
	for i := 0; i < parallelism; i++ {
		go s.doPull(repo, wg, layerCh, resultCh)
	}
	for _, layer := range layers {
		layerCh <- layer
	}
	var result error
	for i := 0; i < len(layers); i++ {
		err := <-resultCh
		if err != nil {
			glog.Warningf("pulling %s: %v", repo, err)
			result = multierror.Append(result, err)
		}
	}
	glog.V(5).Infof("pulling %s: closing sending channel", repo)
	close(layerCh)
	glog.V(5).Infof("pulling %s: waiting for workers to finish", repo)
	wg.Wait()
	return result
}

func (s *Store) Pull(image string) (string, error) {
	repo, ref, err := util.ParseImageSpec(image)
	if err != nil {
		return "", err
	}
	mfest, err := manifest.Fetch(s.reg, repo, ref)
	if err != nil {
		return "", fmt.Errorf("retrieving manifest for %s: %v", image, err)
	}
	err = s.pullLayers(repo, mfest)
	if err != nil {
		return "", fmt.Errorf("pulling layers for %s: %v", image, err)
	}
	err = mfest.Save(s.manifestDir)
	if err != nil {
		return "", fmt.Errorf("saving manifest for %s: %v", image, err)
	}
	imageID := mfest.ID()
	configPath := filepath.Join(s.configDir, imageID)
	if _, err = os.Stat(configPath); err != nil {
		err = s.saveConfig(mfest, configPath)
		if err != nil {
			return "", fmt.Errorf("saving config for %s: %v", image, err)
		}
	}
	return imageID, nil
}

func (s *Store) Unpack(image, dest string) error {
	repo, ref, err := util.ParseImageSpec(image)
	if err != nil {
		return err
	}
	glog.V(2).Infof("image %s is %s:%s", image, repo, ref)
	mfest, err := manifest.Load(s.reg, s.manifestDir, repo, ref)
	if err != nil {
		return err
	}
	for _, layer := range mfest.Layers() {
		dgest := layer.Digest.Encoded()
		err = s.unpackLayer(dgest, dest, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) unpackLayer(dgest, into string, atomic bool) error {
	glog.V(1).Infof("unpacking layer %s into %s", dgest, into)
	path := filepath.Join(s.layerDir, dgest)
	reader, err := os.Open(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	dest := into
	if atomic {
		clean := filepath.Clean(into)
		dir := filepath.Dir(clean)
		base := filepath.Base(clean)
		tmpdir := filepath.Join(dir, "."+base)
		err := os.Mkdir(tmpdir, 0755)
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpdir)
		dest = tmpdir
	}
	err = archive.Untar(reader, dest, &archive.TarOptions{
		NoLchown: true,
		InUserNS: true,
	})
	if atomic {
		err = os.Rename(dest, into)
		if err != nil {
			return err
		}
	}
	return nil
}

func randomString() string {
	var err error
	for retries := 0; retries < maxRetries; retries++ {
		var n *big.Int
		n, err = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			time.Sleep((1 << retries) * 100 * time.Millisecond)
			continue
		}
		i := n.Uint64()
		return strconv.FormatUint(i, 36)
	}
	glog.Fatalf("generating random string: %v", err)
	return ""
}

// When mounting an overlayfs, we need to make sure we are not exceeding the
// maximum length for arguments. We create random short links pointing to the
// layer directories.
func (s *Store) createShortLink(path string) error {
	savedLink := path + ".link"
	_, err := os.Stat(savedLink)
	if err == nil {
		// There is already a link.
		return nil
	}
	dir := filepath.Dir(path)
	for retries := 0; retries < maxRetries; retries++ {
		link := filepath.Join(dir, randomString())
		err = os.Symlink(path, link)
		if err != nil {
			glog.V(2).Infof("%s exists, generating new short link", link)
		}
		err = util.AtomicWriteFile(savedLink, []byte(filepath.Base(link)), 0644)
		if err != nil {
			os.Remove(link)
			return err
		}
		return nil
	}
	return fmt.Errorf("giving up creating link to %s: %v", path, err)
}

func (s *Store) Mount(image, dest string) error {
	repo, ref, err := util.ParseImageSpec(image)
	if err != nil {
		return err
	}
	glog.V(2).Infof("image %s is %s:%s", image, repo, ref)
	mfest, err := manifest.Load(s.reg, s.manifestDir, repo, ref)
	if err != nil {
		return err
	}
	err = os.MkdirAll(dest, 0755)
	if err != nil {
		return err
	}
	if !util.IsEmptyDir(dest) {
		return fmt.Errorf("mount dir %s is not empty or accessible", dest)
	}
	upper := dest + ".upper"
	if err := os.MkdirAll(upper, 0755); err != nil {
		return err
	}
	if !util.IsEmptyDir(upper) {
		return fmt.Errorf("overlayfs dir %s is not empty or accessible", upper)
	}
	work := dest + ".work"
	if err := os.MkdirAll(work, 0755); err != nil {
		return err
	}
	if !util.IsEmptyDir(upper) {
		return fmt.Errorf("overlayfs dir %s is not empty or accessible", work)
	}
	layers := mfest.Layers()
	ch := make(chan error, len(layers))
	for _, layer := range layers {
		dgst := layer.Digest.Encoded()
		into := filepath.Join(s.overlayDir, dgst)
		go func() {
			_, err := os.Stat(into)
			if err != nil {
				err = s.unpackLayer(dgst, into, true)
			}
			if err == nil {
				err = s.createShortLink(into)
			}
			ch <- err
		}()
	}
	for _ = range layers {
		layerErr := <-ch
		if layerErr != nil {
			err = multierror.Append(err, layerErr)
		}
	}
	if err != nil {
		return err
	}
	layerDirs := []string{}
	for i := len(layers) - 1; i >= 0; i-- {
		dgst := layers[i].Digest.Encoded()
		link := filepath.Join(s.overlayDir, dgst+".link")
		linkToLayer, err := ioutil.ReadFile(link)
		if err != nil {
			return err
		}
		layerDirs = append(layerDirs, string(linkToLayer))
	}
	lowers := strings.Join(layerDirs, ":")
	args := []string{
		"-t",
		"overlay",
		"overlay",
		"-o",
		fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowers, upper, work),
		dest,
	}
	glog.V(2).Infof("mounting overlay with args %v", args)
	cmd := exec.Command("mount", args...)
	cmd.Dir = s.overlayDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting to %s: %v; output: %s", dest, err, output)
	}
	return nil
}

type Config struct {
	Config *container.Config `json:"config"`
}

func (s *Store) saveConfig(mfest *manifest.Manifest, path string) error {
	data, err := mfest.Config()
	if err != nil {
		return err
	}
	cfg := Config{}
	glog.V(5).Infof("%s full config: %s", mfest.ID(), string(data))
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return err
	}
	if cfg.Config == nil {
		return fmt.Errorf("%s: missing config in manifest", mfest.ID())
	}
	buf, err := json.Marshal(cfg.Config)
	if err != nil {
		return err
	}
	glog.V(5).Infof("%s saving container config: %s", mfest.ID(), string(buf))
	return util.AtomicWriteFile(path, buf, 0644)
}

func (s *Store) SaveConfig(image, dest string) error {
	repo, ref, err := util.ParseImageSpec(image)
	if err != nil {
		return err
	}
	mfest, err := manifest.Load(s.reg, s.manifestDir, repo, ref)
	if err != nil {
		return err
	}
	imageID := mfest.ID()
	configPath := filepath.Join(s.configDir, imageID)
	buf, err := ioutil.ReadFile(configPath)
	if err == nil {
		return util.AtomicWriteFile(dest, buf, 0644)
	}
	return s.saveConfig(mfest, dest)
}
