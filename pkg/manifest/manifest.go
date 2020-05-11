package manifest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/elotl/tosi/pkg/registryclient"
	"github.com/elotl/tosi/pkg/util"
	"github.com/golang/glog"
)

type Manifest struct {
	Image      string
	Tag        string
	reg        *registryclient.RegistryClient
	ManifestV1 *schema1.SignedManifest
	ManifestV2 *schema2.DeserializedManifest
}

func Fetch(reg *registryclient.RegistryClient, image, tag string) (*Manifest, error) {
	manifest := Manifest{
		Image: image,
		Tag:   tag,
		reg:   reg,
	}
	manifestv2, err := reg.ManifestV2(image, tag)
	if err != nil {
		glog.V(2).Infof("error retrieving v2 manifest: %v, trying v1", err)
	}
	if err != nil || manifestv2.Versioned.SchemaVersion == 1 {
		// Old, v1 manifest.
		manifestv1, err := reg.ManifestV1(image, tag)
		if err != nil {
			return nil, err
		}
		manifest.ManifestV1 = manifestv1
	}
	manifest.ManifestV2 = manifestv2
	return &manifest, nil
}

func Load(reg *registryclient.RegistryClient, dir, image, tag string) (*Manifest, error) {
	manifest := Manifest{
		Image: image,
		Tag:   tag,
		reg:   reg,
	}
	link := filepath.Join(dir, image+":"+tag)
	glog.V(2).Infof("loading manifest for %s:%s from %s", image, tag, dir)
	buf, err := ioutil.ReadFile(link)
	if err != nil {
		return nil, fmt.Errorf("loading %s/%s:%s: %v", dir, image, tag, err)
	}
	v1 := schema1.SignedManifest{}
	err = json.Unmarshal(buf, &v1)
	if err == nil {
		glog.V(5).Infof("%s:%s found v1 manifest", image, tag)
		manifest.ManifestV1 = &v1
		return &manifest, nil
	}
	glog.V(5).Infof("%s:%s loading v1 manifest: %v, trying v2", image, tag, err)
	v2 := schema2.DeserializedManifest{}
	err = json.Unmarshal(buf, &v2)
	if err == nil {
		glog.V(5).Infof("%s:%s found v2 manifest", image, tag)
		manifest.ManifestV2 = &v2
		return &manifest, nil
	}
	glog.V(5).Infof("%s:%s loading v2 manifest: %v", image, tag, err)
	return nil, fmt.Errorf("loading %s/%s:%s: %v", dir, image, tag, err)
}

func (m *Manifest) v1Config() ([]byte, error) {
	if len(m.ManifestV1.History) < 1 {
		return nil, fmt.Errorf("no config found")
	}
	v1comp := m.ManifestV1.History[0].V1Compatibility
	return []byte(v1comp), nil
}

func (m *Manifest) v2Config() ([]byte, error) {
	return m.reg.GetBlob(m.Image, m.ManifestV2.Config)
}

func (m *Manifest) Config() ([]byte, error) {
	if m.ManifestV1 != nil {
		return m.v1Config()
	} else if m.ManifestV2 != nil {
		return m.v2Config()
	}
	panic("no manifest available")
}

func (m *Manifest) v1Layers() []distribution.Descriptor {
	v1refs := m.ManifestV1.References()
	refs := make([]distribution.Descriptor, 0, len(v1refs))
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
	return refs
}

func (m *Manifest) v2Layers() []distribution.Descriptor {
	return m.ManifestV2.Layers
}

func (m *Manifest) Layers() []distribution.Descriptor {
	if m.ManifestV1 != nil {
		return m.v1Layers()
	} else if m.ManifestV2 != nil {
		return m.v2Layers()
	}
	panic("no manifest available")
}

func (m *Manifest) v1ID() string {
	name := "v1:" + m.ManifestV1.Name + ":" + m.ManifestV1.Tag
	return strings.ReplaceAll(name, "/", ":")
}

func (m *Manifest) v2ID() string {
	return "v2:" + m.ManifestV2.Config.Digest.String()
}

func (m *Manifest) ID() string {
	if m.ManifestV1 != nil {
		return m.v1ID()
	} else if m.ManifestV2 != nil {
		return m.v2ID()
	}
	panic("no manifest available")
}

func (m *Manifest) Save(dir string) error {
	var buf []byte
	var err error
	if m.ManifestV1 != nil {
		_, buf, err = m.ManifestV1.Payload()
	} else {
		_, buf, err = m.ManifestV2.Payload()
	}
	if err != nil {
		return err
	}
	// Save manifest if it does not exist.
	fileName := m.ID()
	path := filepath.Join(dir, fileName)
	if _, err = os.Stat(path); err != nil {
		err = os.MkdirAll(filepath.Dir(path), 0755)
		if err != nil {
			return err
		}
		err = util.AtomicWriteFile(path, buf, 0644)
		if err != nil {
			return err
		}
	}
	// Create a link with the image name pointing to the manifest. If the link
	// already exists, it will be updated.
	link := filepath.Join(dir, m.Image+":"+m.Tag)
	linkDir := filepath.Dir(link)
	err = os.MkdirAll(linkDir, 0755)
	if err != nil {
		return err
	}
	// Create a relative link, so that it does not depend on absolute paths.
	// This helps if the cache is moved to new directory.
	rel, err := filepath.Rel(linkDir, dir)
	if err != nil {
		return err
	}
	target := filepath.Join(rel, fileName)
	_ = os.Remove(link)
	err = os.Symlink(target, link)
	if err != nil {
		return err
	}
	return nil
}
