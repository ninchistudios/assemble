// Package web wires the HTTP routes, templates and middleware.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/ninchistudios/assemble/internal/config"
	"github.com/ninchistudios/assemble/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	cfg     config.Config
	db      *store.Store
	tpl     *template.Template
	mux     *http.ServeMux
	handler http.Handler
	etags   map[string]string // /static/<file> -> content hash
}

func New(cfg config.Config, db *store.Store) (*Server, error) {
	tpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	etags, err := staticETags()
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:   cfg,
		db:    db,
		tpl:   tpl,
		mux:   http.NewServeMux(),
		etags: etags,
	}
	s.routes()
	s.handler = securityHeaders(s.mux)
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleHome)
	s.mux.HandleFunc("GET /style", s.handleStyle)

	// Unauthenticated. Liveness plus the build, so a deploy can be confirmed
	// from outside without signing in.
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok " + s.cfg.Version + "\n"))
	})
	s.mux.Handle("GET /static/", s.staticHandler())

	// Everything else is a styled 404 rather than the mux's plain-text one.
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.render(w, http.StatusNotFound, "message.html", page{Title: "Not found", Message: "There's nothing here."})
	})
}

// page is the data every template receives.
type page struct {
	Title   string // suffix in the browser tab; empty on the home page
	Version string
	Message string
}

// securityHeaders sets the headers every response carries, error responses
// included. Setting them in a wrapper, before the handler runs, is what makes
// "every response" true: a header set after WriteHeader is silently dropped,
// which is how error pages lose them when each handler sets its own.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// No third-party anything: no CDN, no font service, no analytics.
		h.Set("Content-Security-Policy",
			"default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		// Group members' names and availability, behind a session cookie,
		// passing through a reverse proxy. Nothing in between may cache it.
		// Static assets override this with a revalidating policy.
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// render writes a page with its status. Taking the status as a parameter keeps
// WriteHeader in one place, after the content type is set.
func (s *Server) render(w http.ResponseWriter, status int, name string, p page) {
	p.Version = s.cfg.Version
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.tpl.ExecuteTemplate(w, name, p); err != nil {
		// Status and headers are already on the wire, so this cannot become a
		// 500. Log it; the client gets a truncated page.
		log.Printf("render %s: %v", name, err)
	}
}

// staticETags hashes each embedded asset once at startup. embed.FS reports a
// zero ModTime, so http.FileServerFS emits neither Last-Modified nor ETag and
// the browser refetches every asset in full on every navigation. A content
// hash is a validator that survives restarts and changes exactly when the file
// does, which a build timestamp would not.
func staticETags() (map[string]string, error) {
	tags := map[string]string{}
	err := fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := staticFS.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		tags["/"+path] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	return tags, err
}

// staticHandler serves the embedded assets with a validator, letting
// http.ServeContent answer a matching If-None-Match with a bodyless 304.
func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(staticFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tag, ok := s.etags[r.URL.Path]; ok {
			w.Header().Set("ETag", tag)
			// Revalidate every time rather than trusting a max-age: a stale
			// stylesheet surviving a redeploy looks like a rendering bug and is
			// very hard to recognise as a caching one.
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "home.html", page{})
}

// handleStyle shows the palette, type and icons on a real device. See
// docs/ui.md.
func (s *Server) handleStyle(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "style.html", page{Title: "Style"})
}
