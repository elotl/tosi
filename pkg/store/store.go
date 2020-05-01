package store

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/archive"
	"github.com/elotl/tosi/pkg/manifest"
	"github.com/elotl/tosi/pkg/registryclient"
	"github.com/elotl/tosi/pkg/util"
	"github.com/golang/glog"
)

type Store struct {
	BaseDir     string
	layerDir    string
	configDir   string
	manifestDir string
	reg         *registryclient.RegistryClient
}

func NewStore(basedir string, reg *registryclient.RegistryClient) (*Store, error) {
	layerdir := filepath.Join(basedir, "layers")
	configdir := filepath.Join(basedir, "configs")
	manifestdir := filepath.Join(basedir, "manifests")
	for _, dir := range []string{layerdir, configdir, manifestdir} {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return nil, fmt.Errorf("creating %s: %v", dir, err)
		}
	}
	return &Store{
		BaseDir:     basedir,
		layerDir:    layerdir,
		configDir:   configdir,
		manifestDir: manifestdir,
		reg:         reg,
	}, nil
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
	for _, layer := range mfest.Layers() {
		_, err := s.reg.SaveBlob(repo, s.layerDir, layer)
		if err != nil {
			return "", fmt.Errorf(
				"downloading layer %v for %s: %v", layer, image, err)
		}
	}
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
