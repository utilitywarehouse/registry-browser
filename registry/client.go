package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema2"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ImageConfig is the configuration object for a container
// runtime uses this config to set up the container.
type ImageConfig struct {
	Architecture    string             `json:"architecture"`
	Config          *ImageConfigConfig `json:"config"`
	Container       string             `json:"container"`
	ContainerConfig *ImageConfigConfig `json:"container_config"`
	Created         time.Time          `json:"created"`
	DockerVersion   string             `json:"docker_version"`
	History         []History          `json:"history"`
	ID              string             `json:"id"`
	OS              string             `json:"os"`
	Parent          string             `json:"parent"`
	Size            int64              `json:"Size"`
}

// ImageConfigConfig is the configuration of a layer in ImageConfig
type ImageConfigConfig struct {
	ArgsEscaped  bool                         `json:"ArgsEscaped"`
	AttachStderr bool                         `json:"AttachStderr"`
	AttachStdin  bool                         `json:"AttachStdin"`
	AttachStdout bool                         `json:"AttachStdout"`
	Cmd          []string                     `json:"Cmd"`
	DomainName   string                       `json:"Domainname"`
	Entrypoint   []string                     `json:"Entrypoint"`
	Env          []string                     `json:"Env"`
	Hostname     string                       `json:"Hostname"`
	Image        string                       `json:"Image"`
	Labels       map[string]string            `json:"Labels"`
	OnBuild      []string                     `json:"OnBuild"`
	OpenStdin    bool                         `json:"OpenStdin"`
	StdinOnce    bool                         `json:"StdinOnce"`
	Tty          bool                         `json:"Tty"`
	User         string                       `json:"User"`
	Volumes      map[string]map[string]string `json:"Volumes"`
	WorkingDir   string                       `json:"WorkingDir"`
}

// History of each layer. The array is ordered from first to last
type History struct {
	Created    *time.Time `json:"created"`
	CreatedBy  *string    `json:"created_by"`
	Author     *string    `json:"author"`
	Comment    *string    `json:"comment"`
	EmptyLayer *bool      `json:"empty_layer"`
}

// ManifestInfo is a summary of the manifests retrieved from the registry for
// all supported schema versions
type ManifestInfo struct {
	Digest    string
	Config    *ImageConfig
	Layers    []distribution.Descriptor
	Manifests []manifestlist.ManifestDescriptor
	Formats   []string
}

// Client for a docker registry
type Client struct {
	httpClient  *http.Client
	registryURL string
}

// New returns a new client
func New(registryURL string) (*Client, error) {
	return &Client{
		httpClient:  &http.Client{Timeout: 1 * time.Minute},
		registryURL: registryURL,
	}, nil
}

// ManifestInfo retrieves manifest information for a given name+reference from all
// supported schemas
func (c *Client) ManifestInfo(name, reference string) (*ManifestInfo, error) {
	var manifestIno = new(ManifestInfo)

	// Retrieve the manifest in multiple formats so that we can provide the
	// most information possible to the client. If the reference doesn't support a
	// particular schema then the registry will return the manifest in a schema it
	// does support. That's why we check the version and only add it to the
	// list of formats if it matches the expected values.

	// OCI Image Specification is compatible with docker image specs
	//
	// `application/vnd.oci.image.index.v1+json` =~ `application/vnd.docker.distribution.manifest.list.v2+json`
	// `application/vnd.oci.image.manifest.v1+json` =~ `application/vnd.docker.distribution.manifest.v2+json`
	// https://github.com/opencontainers/image-spec/blob/main/media-types.md#compatibility-matrix

	manifestListSchema2, digestListSchema2, err := c.manifestListSchema2(name, reference)
	if err != nil {
		return nil, err
	}
	if manifestListSchema2 != nil && manifestListSchema2.SchemaVersion == 2 &&
		(manifestListSchema2.MediaType == manifestlist.MediaTypeManifestList ||
			manifestListSchema2.MediaType == imagespec.MediaTypeImageIndex) {

		manifestIno.Digest = digestListSchema2
		manifestIno.Manifests = manifestListSchema2.Manifests
		manifestIno.Formats = append(manifestIno.Formats, manifestListSchema2.MediaType)
		return manifestIno, nil
	}

	manifestSchema2, digestSchema2, err := c.manifestSchema2(name, reference)
	if err != nil {
		return nil, err
	}
	if manifestSchema2 == nil {
		return manifestIno, nil
	}

	if manifestSchema2.SchemaVersion == 2 &&
		(manifestSchema2.MediaType == schema2.MediaTypeManifest ||
			manifestSchema2.MediaType == imagespec.MediaTypeImageManifest) {

		manifestIno.Digest = digestSchema2
		manifestIno.Layers = manifestSchema2.Layers
		manifestIno.Formats = append(manifestIno.Formats, manifestSchema2.MediaType)
	}

	if manifestSchema2.Config.Digest == "" {
		return manifestIno, nil
	}

	config, err := c.ImageConfig(name, string(manifestSchema2.Config.Digest))
	if err != nil {
		return nil, err
	}
	manifestIno.Config = config

	return manifestIno, nil
}

// manifestRequest unmarshals the manifest for name+reference into the provided
// interface in the requested format. Returns the manifest digest.
func (c *Client) manifestRequest(name, reference string, manifest interface{}, mediaType ...string) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v2/%s/manifests/%s", c.registryURL, name, reference), nil)
	if err != nil {
		return "", err
	}

	req.Header["Accept"] = mediaType

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if err := json.Unmarshal(body, manifest); err != nil {
		return "", err
	}

	return resp.Header.Get("Docker-Content-Digest"), nil
}

func (c *Client) blobRequest(name, reference string, blob interface{}) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v2/%s/blobs/%s", c.registryURL, name, reference), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, blob); err != nil {
		return err
	}

	return nil
}

// manifestSchema2 retrieves the manifests for the name+reference for v2
// manifest schema 2
func (c *Client) manifestSchema2(name, reference string) (*schema2.Manifest, string, error) {
	manifest := &schema2.Manifest{}
	digest, err := c.manifestRequest(name, reference, manifest,
		schema2.MediaTypeManifest, imagespec.MediaTypeImageManifest)
	if err != nil {
		return nil, "", err
	}

	return manifest, digest, nil
}

// manifestListSchema2 retrieves the manifests for the name+reference for v2 manifest schema 2
func (c *Client) manifestListSchema2(name, reference string) (*manifestlist.ManifestList, string, error) {
	manifestList := &manifestlist.ManifestList{}
	digest, err := c.manifestRequest(name, reference, manifestList,
		manifestlist.MediaTypeManifestList, imagespec.MediaTypeImageIndex)
	if err != nil {
		return nil, "", err
	}

	return manifestList, digest, nil
}

// ImageConfig retrieves the image configuration JSON document
func (c *Client) ImageConfig(name, reference string) (*ImageConfig, error) {
	config := &ImageConfig{}
	err := c.blobRequest(name, reference, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
