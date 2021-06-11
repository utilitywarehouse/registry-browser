package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/distribution/distribution/v3/reference"
	"github.com/gorilla/mux"
	"github.com/opencontainers/go-digest"
	"github.com/utilitywarehouse/registry-browser/registry"
	"github.com/utilitywarehouse/registry-browser/s3"
)

// templateMinus subtracts two ints within a template
func templateMinus(a, b int) int {
	return a - b
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
		"minus":       templateMinus,
		"breadCrumbs": templateBreadCrumbs,
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
	m.HandleFunc("/repository/{name:"+reference.NameRegexp.String()+"}/manifests/{reference:"+reference.TagRegexp.String()+"|"+digest.DigestRegexp.String()+"}", srv.handleManifests)
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

	tags, err := s.s3.List(filepath.Join(name, "_manifests/tags"))
	if err != nil {
		http.Error(w, "error retrieving tags", http.StatusInternalServerError)
		log.Printf("error: %s", err)
		return
	}

	var data struct {
		Name  string
		Repos []string
		Tags  []string
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
	for _, c := range manifestInfo.History {
		if created.IsZero() && !c.Created.IsZero() {
			created = c.Created
			break
		}
	}

	// Calculate the total image size
	var imageSize int64
	for _, l := range manifestInfo.Layers {
		imageSize = imageSize + l.Size
	}
	if imageSize == 0 {
		for _, c := range manifestInfo.History {
			imageSize = imageSize + c.Size
		}
	}

	// Count the layers
	layersCount := len(manifestInfo.Layers)
	if layersCount == 0 {
		layersCount = len(manifestInfo.History)
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
