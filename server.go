package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/gorilla/mux"
	ocDigest "github.com/opencontainers/go-digest"
	"github.com/utilitywarehouse/registry-browser/registry"
	"github.com/utilitywarehouse/registry-browser/s3"
)

// templatePlus subtracts two ints within a template
func templatePlus(a, b int) int {
	return a + b
}

// breadCrumb facilitates building breadcrumb style navigation menus of the
// form: a / b / c, where 'c' is a hyperlink to 'a/b/c', 'b' to 'a/b' and 'a' to
// 'a'
type breadCrumb struct {
	Segment string
	Path    string
}

// templateBreadCrumbs creates breadcrumbs for a repository name within a
// template
func templateBreadCrumbs(name string) []breadCrumb {
	bc := []breadCrumb{}
	segments := strings.Split(name, "/")

	for i := 0; i < len(segments); i++ {
		e := breadCrumb{Segment: segments[i], Path: strings.Join(segments[0:i+1], "/")}
		bc = append(bc, e)
	}

	return bc
}

// server implements http.Handler, it serves the browser
type server struct {
	http.Handler
	r    *registry.Client
	s3   *s3.Client
	tmpl *template.Template
}

// newServer returns a new server
func newServer(r *registry.Client, s *s3.Client) (*server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"plus":        templatePlus,
		"breadCrumbs": templateBreadCrumbs,
		"join":        strings.Join,
	}).ParseFiles("./templates/manifests.html", "./templates/list.html")
	if err != nil {
		return nil, err
	}

	srv := &server{
		r:    r,
		s3:   s,
		tmpl: tmpl,
	}

	m := mux.NewRouter()
	m.HandleFunc("/repository/{name:"+reference.NameRegexp.String()+"}/manifests/{reference:"+reference.TagRegexp.String()+"|"+ocDigest.DigestRegexp.String()+"}", srv.handleManifests)
	m.HandleFunc("/repository/{name:"+reference.NameRegexp.String()+"}", srv.handleList)
	m.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	m.HandleFunc("/", srv.handleList)

	srv.Handler = m

	return srv, nil
}

// handleList handles requests for /repository/{name} where name is a valid
// repository path. It displays the subpaths of that path and/or any tags if
// it's a repository.
func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	items, err := s.s3.List(name)
	if err != nil {
		http.Error(w, "error retrieving repositories", http.StatusInternalServerError)
		log.Printf("error: %s", err)
		return
	}

	var repos []string
	for _, item := range items {
		// Remove _layers and _manifests, which aren't repositories or part of
		// a repository path
		if item != "_layers" && item != "_manifests" {
			repos = append(repos, item)
		}
	}

	objects, err := s.s3.ListTagObjectsWithMetadata(filepath.Join(name, "_manifests/tags"))
	if err != nil {
		http.Error(w, "error retrieving tag metadata", http.StatusInternalServerError)
		log.Printf("error: %s", err)
		return
	}

	tags := parseTagsInfo(objects)

	var data struct {
		Name  string
		Repos []string
		Tags  map[string]*tagInfo
	}

	data.Name = name
	data.Repos = repos
	data.Tags = tags

	rendered := &bytes.Buffer{}
	if err := s.tmpl.ExecuteTemplate(rendered, "list.html", data); err != nil {
		http.Error(w, "error rendering template", http.StatusInternalServerError)
		log.Printf("error: %s", err)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := rendered.WriteTo(w); err != nil {
		log.Printf("error: %s", err)
	}
}

type tagInfo struct {
	Tag        string     // tag of the image
	ModifiedAt time.Time  // when was the tag Last Modified
	Manifest   manifest   // current manifest details
	Index      []manifest // history of manifests on this tag
}

type manifest struct {
	SHA256     string
	ModifiedAt time.Time
	Tags       []string // current tags
}

func parseTagsInfo(objects []s3.S3ObjectInfo) map[string]*tagInfo {
	tags := make(map[string]*tagInfo)
	for _, obj := range objects {
		// check if its a tag path /current/link
		if strings.HasSuffix(obj.Key, "/current/link") {
			parts := strings.Split(obj.Key, "/")
			if len(parts) < 3 {
				continue
			}
			tag := parts[len(parts)-3] // tag is third from the end tag path

			_, ok := tags[tag]
			if !ok {
				tags[tag] = &tagInfo{Tag: tag, ModifiedAt: obj.LastModified}
			}
			if obj.LastModified.After(tags[tag].ModifiedAt) {
				fmt.Println("why??", obj.LastModified, tags[tag].ModifiedAt)
				// images[tag].CreatedAt = obj.LastModified
			}
			continue
		}

		// check if its index key path
		if strings.Contains(obj.Key, "index/sha256/") {
			parts := strings.Split(obj.Key, "/")
			if len(parts) < 5 {
				continue
			}
			tag := parts[len(parts)-5]    // tag is 5th from the end in index path
			sha256 := parts[len(parts)-2] // index is 2nd from the end in index path

			_, ok := tags[tag]
			if !ok {
				tags[tag] = &tagInfo{Tag: tag}
			}
			manifest := manifest{SHA256: sha256, ModifiedAt: obj.LastModified}

			tags[tag].Index = append(tags[tag].Index, manifest)

			// update current manifests if latest found
			if obj.LastModified.After(tags[tag].Manifest.ModifiedAt) {
				tags[tag].Manifest = manifest
			}
		}
	}

	// sort each index and only keep latest 10
	for _, tag := range tags {
		// sort in desc order of CreatedAt
		slices.SortFunc(tag.Index, func(a, b manifest) int {
			return b.ModifiedAt.Compare(a.ModifiedAt)
		})
		if len(tag.Index) > 10 {
			tag.Index = slices.Delete(tag.Index, 10, len(tag.Index))
		}
	}

	// Loop through manifests history (index) and find current tags
	for _, tag := range tags {
		for i := range tag.Index {
			for _, LookupTag := range tags {
				if tag.Index[i].SHA256 == LookupTag.Manifest.SHA256 {
					tag.Index[i].Tags = append(tag.Index[i].Tags, LookupTag.Tag)
				}
			}
		}
	}
	return tags
}

// handleManifests handles requests for /repository/{name}/manifests/{reference}
// where name is the name of a repository and reference is a tag or digest. It
// displays manifest information.
func (s *server) handleManifests(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	reference := mux.Vars(r)["reference"]

	manifestInfo, err := s.r.ManifestInfo(name, reference)
	if err != nil {
		http.Error(w, "error fetching manifest", http.StatusInternalServerError)
		log.Printf("error: %s", err)
		return
	}

	// Find created time
	var created time.Time
	if manifestInfo.Config != nil {
		created = manifestInfo.Config.Created
	}
	// Calculate the total image size
	var imageSize int64
	for _, l := range manifestInfo.Layers {
		imageSize = imageSize + l.Size
	}

	// Count the layers
	layersCount := len(manifestInfo.Layers)
	if layersCount == 0 && manifestInfo.Config != nil {
		layersCount = len(manifestInfo.Config.History)
	}

	var data struct {
		Name      string
		Reference string
		Created   time.Time
		Size      int64
		Layers    int
		Manifest  *registry.ManifestInfo
	}
	data.Name = name
	data.Reference = reference
	data.Created = created
	data.Size = imageSize
	data.Layers = layersCount
	data.Manifest = manifestInfo

	rendered := &bytes.Buffer{}
	if err := s.tmpl.ExecuteTemplate(rendered, "manifests.html", data); err != nil {
		http.Error(w, "error rendering template", http.StatusInternalServerError)
		log.Printf("error: %s", err)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := rendered.WriteTo(w); err != nil {
		log.Printf("error: %s", err)
	}
}
