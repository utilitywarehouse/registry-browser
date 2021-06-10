package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema1"
	"github.com/distribution/distribution/v3/manifest/schema2"
)

// V1Compatibility is the v1 schema History
type V1Compatibility struct {
	Architecture    string                 `json:"architecture"`
	Config          *V1CompatibilityConfig `json:"config"`
	Container       string                 `json:"container"`
	ContainerConfig *V1CompatibilityConfig `json:"container_config"`
	Created         time.Time              `json:"created"`
	DockerVersion   string                 `json:"docker_version"`
	ID              string                 `json:"id"`
	OS              string                 `json:"os"`
	Parent          string                 `json:"parent"`
	Size            int64                  `json:"Size"`
	Throwaway       bool                   `json:"throwaway"`
}

// V1CompatibilityConfig is the configuration of a layer in V1Compatibility
type V1CompatibilityConfig struct {
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

// Client for a docker registry
type Client struct {
	httpClient  *http.Client
	registryURL string
}

// New returns a new client
func New(registryURL string) (*Client, error) {
	return &Client{
		httpClient:  &http.Client{},
		registryURL: registryURL,
	}, nil
}

// ManifestInfo is a summary of the manifests retrieved from the registry for
// all supported schema versions
type ManifestInfo struct {
	Digest    string
	History   []*V1Compatibility
	Layers    []distribution.Descriptor
	Manifests []manifestlist.ManifestDescriptor
	Formats   []string
}

// ManifestInfo retrieves manifest information for a given name+reference from all
// supported schemas
func (c *Client) ManifestInfo(name, reference string) (*ManifestInfo, error) {
	var formats []string

	// Retrieve the manifest in multiple formats so that we can provide the
	// most information possible to the client. If the reference doesn't support a
	// particular schema then the registry will return the manifest in a schema it
	// does support. That's why we check the version and only add it to the
	// list of formats if it matches the expected values.
	manifestSchema1, digestSchema1, err := c.manifestSchema1(name, reference)
	if err != nil {
		return nil, err
	}
	if manifestSchema1.SchemaVersion == 1 {
		formats = append(formats, schema1.MediaTypeManifest)
	}

	manifestSchema2, digestSchema2, err := c.manifestSchema2(name, reference)
	if err != nil {
		return nil, err
	}
	if manifestSchema2.SchemaVersion == 2 && manifestSchema2.MediaType == schema2.MediaTypeManifest {
		formats = append(formats, schema2.MediaTypeManifest)
	}

	manifestListSchema2, digestListSchema2, err := c.manifestListSchema2(name, reference)
	if err != nil {
		return nil, err
	}
	if manifestListSchema2.SchemaVersion == 2 && manifestListSchema2.MediaType == manifestlist.MediaTypeManifestList {
		formats = append(formats, manifestlist.MediaTypeManifestList)
	}

	digest := digestSchema2
	if digest == "" {
		digest = digestSchema1
	}
	if len(manifestListSchema2.Manifests) > 0 {
		digest = digestListSchema2
	}

	history := []*V1Compatibility{}
	for _, h := range manifestSchema1.History {
		c := &V1Compatibility{}
		if err := json.Unmarshal([]byte(h.V1Compatibility), c); err != nil {
			return nil, err
		}
		history = append(history, c)
	}

	return &ManifestInfo{
		Digest:    digest,
		Formats:   formats,
		History:   history,
		Layers:    manifestSchema2.Layers,
		Manifests: manifestListSchema2.Manifests,
	}, nil
}

// manifestRequest unmarshals the manifest for name+reference into the provided
// interface in the requested format. Returns the manifest digest.
func (c *Client) manifestRequest(name, reference, format string, manifest interface{}) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v2/%s/manifests/%s", c.registryURL, name, reference), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", fmt.Sprintf("application/vnd.docker.distribution.%s+json", format))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if err := json.Unmarshal(body, manifest); err != nil {
		return "", err
	}

	digest := resp.Header.Get("Docker-Content-Digest")

	return digest, nil

}

// manifestSchema1 retrieves the manifests for the name+reference for v2 manifest schema 1
func (c *Client) manifestSchema1(name, reference string) (*schema1.Manifest, string, error) {
	manifest := &schema1.Manifest{}
	digest, err := c.manifestRequest(name, reference, "manifest.v1", manifest)
	if err != nil {
		return nil, "", err
	}

	return manifest, digest, nil
}

// manifestSchema2 retrieves the manifests for the name+reference for v2
// manifest schema 2
func (c *Client) manifestSchema2(name, reference string) (*schema2.Manifest, string, error) {
	manifest := &schema2.Manifest{}
	digest, err := c.manifestRequest(name, reference, "manifest.v2", manifest)
	if err != nil {
		return nil, "", err
	}

	return manifest, digest, nil
}

// manifestListSchema2 retrieves the manifests for the name+reference for v2 manifest schema 2
func (c *Client) manifestListSchema2(name, reference string) (*manifestlist.ManifestList, string, error) {
	manifestList := &manifestlist.ManifestList{}
	digest, err := c.manifestRequest(name, reference, "manifest.list.v2", manifestList)
	if err != nil {
		return nil, "", err
	}

	return manifestList, digest, nil
}
