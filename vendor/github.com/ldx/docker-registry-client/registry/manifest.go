package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime"

	manifestlist "github.com/docker/distribution/manifest/manifestlist"
	manifestV1 "github.com/docker/distribution/manifest/schema1"
	manifestV2 "github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	imageV1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func (registry *Registry) Manifest(repository, reference string) (*manifestV1.SignedManifest, error) {
	url := registry.url("/v2/%s/manifests/%s", repository, reference)
	registry.Logf("registry.manifest.get url=%s repository=%s reference=%s", url, repository, reference)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", manifestV1.MediaTypeManifest)
	req.Header.Add("Accept", manifestV1.MediaTypeSignedManifest)
	resp, err := registry.Client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	signedManifest := &manifestV1.SignedManifest{}
	err = signedManifest.UnmarshalJSON(body)
	if err != nil {
		return nil, err
	}

	return signedManifest, nil
}

func (registry *Registry) ManifestV2(repository, reference string) (*manifestV2.DeserializedManifest, error) {
	url := registry.url("/v2/%s/manifests/%s", repository, reference)
	registry.Logf("registry.manifest.get url=%s repository=%s reference=%s", url, repository, reference)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", manifestV2.MediaTypeManifest)
	req.Header.Add("Accept", manifestlist.MediaTypeManifestList)
	resp, err := registry.Client.Do(req)
	if err != nil {
		return nil, err
	}
	contentType := resp.Header.Get("Content-Type")

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if contentType == manifestlist.MediaTypeManifestList {
		index := imageV1.Index{}
		err = json.Unmarshal(body, &index)
		if err != nil {
			return nil, err
		}
		if index.SchemaVersion != 2 {
			return nil, fmt.Errorf(
				"Invalid schema version in manifest response: %s", string(body))
		}
		for _, m := range index.Manifests {
			if m.Platform == nil ||
				m.Platform.OS != runtime.GOOS ||
				m.Platform.Architecture != runtime.GOARCH {
				continue
			}
			return registry.ManifestV2(repository, m.Digest.String())
		}
		return nil, fmt.Errorf("Arch %q OS %q not found in index %s",
			runtime.GOARCH, runtime.GOOS, string(body))
	}

	if contentType != manifestV2.MediaTypeManifest {
		return nil, fmt.Errorf(
			"Invalid Content-Type in manifest response: %q", contentType)
	}

	deserialized := &manifestV2.DeserializedManifest{}
	err = deserialized.UnmarshalJSON(body)
	if err != nil {
		return nil, err
	}
	return deserialized, nil
}

func (registry *Registry) ManifestDigest(repository, reference string) (digest.Digest, error) {
	url := registry.url("/v2/%s/manifests/%s", repository, reference)
	registry.Logf("registry.manifest.head url=%s repository=%s reference=%s", url, repository, reference)

	resp, err := registry.Client.Head(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", err
	}
	return digest.Parse(resp.Header.Get("Docker-Content-Digest"))
}

func (registry *Registry) DeleteManifest(repository string, digest digest.Digest) error {
	url := registry.url("/v2/%s/manifests/%s", repository, digest)
	registry.Logf("registry.manifest.delete url=%s repository=%s reference=%s", url, repository, digest)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := registry.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	return nil
}

func (registry *Registry) PutManifest(repository, reference string, signedManifest *manifestV1.SignedManifest) error {
	url := registry.url("/v2/%s/manifests/%s", repository, reference)
	registry.Logf("registry.manifest.put url=%s repository=%s reference=%s", url, repository, reference)

	body, err := signedManifest.MarshalJSON()
	if err != nil {
		return err
	}

	buffer := bytes.NewBuffer(body)
	req, err := http.NewRequest("PUT", url, buffer)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", manifestV1.MediaTypeManifest)
	resp, err := registry.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	return err
}
