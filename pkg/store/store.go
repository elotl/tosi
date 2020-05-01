package store

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/distribution"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
	"github.com/elotl/tosi/pkg/manifest"
	"github.com/elotl/tosi/pkg/registryclient"
	"github.com/elotl/tosi/pkg/util"
	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"
)

type Store struct {
	BaseDir           string
	layerDir          string
	configDir         string
	manifestDir       string
	parallelDownloads int
	reg               *registryclient.RegistryClient
}

func NewStore(basedir string, parallelism int, reg *registryclient.RegistryClient) (*Store, error) {
	layerdir := filepath.Join(basedir, "layers")
	configdir := filepath.Join(basedir, "configs")
	manifestdir := filepath.Join(basedir, "manifests")
	for _, dir := range []string{layerdir, configdir, manifestdir} {
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
	imageID := mfest.ID()
	configPath := filepath.Join(s.configDir, imageID)
	err = s.saveConfig(mfest, configPath)
	if err != nil {
		return "", fmt.Errorf("saving config for %s: %v", image, err)
	}
	err = mfest.Save(s.manifestDir)
	if err != nil {
		return "", fmt.Errorf("saving manifest for %s: %v", image, err)
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
		dgest := layer.Digest.String()
		path := filepath.Join(s.layerDir, dgest)
		reader, err := os.Open(path)
		if err != nil {
			return err
		}
		defer reader.Close()
		glog.V(1).Infof("unpacking %s %s into %s", image, dgest, dest)
		err = archive.Untar(reader, dest, &archive.TarOptions{NoLchown: true})
		if err != nil {
			return err
		}
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
