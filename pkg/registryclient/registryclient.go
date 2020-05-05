package registryclient

import (
	"bytes"
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
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/golang/glog"
	"github.com/ldx/docker-registry-client/registry"
	"github.com/opencontainers/go-digest"
)

type RegistryClient struct {
	reg                  *registry.Registry
	validateCachedLayers bool
}

func NewRegistryClient(registryURL, username, password string, validate bool) (*RegistryClient, error) {
	url := strings.TrimSuffix(registryURL, "/")
	// Creates a client with a shorter connection timeout, useful inside AWS.
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
	reg := &registry.Registry{
		URL: url,
		Client: &http.Client{
			Transport: transport,
		},
		Logf: registry.Log,
	}
	if err := reg.Ping(); err != nil {
		glog.Warningf("pinging %s failed: %v", registryURL, err)
	}
	return &RegistryClient{
		reg:                  reg,
		validateCachedLayers: validate,
	}, nil
}

func (r *RegistryClient) ManifestV1(image, tag string) (*schema1.SignedManifest, error) {
	return r.reg.Manifest(image, tag)
}

func (r *RegistryClient) ManifestV2(image, tag string) (*schema2.DeserializedManifest, error) {
	return r.reg.ManifestV2(image, tag)
}

func (r *RegistryClient) GetBlob(image string, desc distribution.Descriptor) ([]byte, error) {
	name := desc.Digest.String()
	glog.V(2).Infof("getting image %s blob %s", image, name)
	reader, err := r.reg.DownloadLayer(image, desc.Digest)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var buf bytes.Buffer
	verifier := desc.Digest.Verifier()
	writer := io.MultiWriter(&buf, verifier)
	n, err := io.Copy(writer, reader)
	if err != nil {
		return nil, err
	}
	if n < desc.Size {
		return nil, fmt.Errorf("image %s blob %s: got only %d/%d bytes",
			image, name, n, desc.Size)
	}
	if !verifier.Verified() {
		return nil, fmt.Errorf("saving %s: verifier failed", name)
	}
	return buf.Bytes(), nil
}

func (r *RegistryClient) SaveBlob(image, dir string, desc distribution.Descriptor) (string, error) {
	name := filepath.Join(dir, desc.Digest.Encoded())
	// Check if we already have the blob downloaded.
	if _, err := os.Stat(name); err == nil {
		if !r.validateCachedLayers || isLayerValid(name, desc.Digest) {
			// Blob file already exists.
			glog.V(2).Infof("image %s blob %s already exists", image, name)
			return name, nil
		}
	}
	glog.V(2).Infof("saving image %s blob %s", image, name)
	tmpdir, err := ioutil.TempDir(dir, "tmp-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpdir)
	tmpname := filepath.Join(tmpdir, desc.Digest.Encoded())
	reader, err := r.reg.DownloadLayer(image, desc.Digest)
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
		return "", fmt.Errorf(
			"saving %s: wrote only %d/%d bytes", name, n, desc.Size)
	}
	glog.V(5).Infof("%s size: %d bytes", name, n)
	if !verifier.Verified() {
		return "", fmt.Errorf("%s: verifier failed", name)
	}
	err = os.Rename(tmpname, name)
	if err != nil {
		return "", err
	}
	glog.V(2).Infof("%s saved blob", name)
	return name, nil
}

func isLayerValid(name string, dgst digest.Digest) bool {
	glog.V(2).Infof("checking layer %s %s", name, dgst.String())
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
