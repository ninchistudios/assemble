// Package web wires the HTTP routes, templates and middleware.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ninchistudios/assemble/internal/auth"
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
	google  auth.Google
	tpl     *template.Template
	mux     *http.ServeMux
	handler http.Handler
	etags   map[string]string // /static/<file> -> content hash

	// refusals rate-limits the log lines strangers can cause.
	refusals *logLimiter
}

// maxBody caps a request body. The largest real form is the group settings,
// a few hundred bytes.
const maxBody = 64 << 10

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
		cfg: cfg,
		db:  db,
		google: auth.Google{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.RedirectURL(),
		},
		tpl:      tpl,
		mux:      http.NewServeMux(),
		etags:    etags,
		refusals: newLogLimiter(20, time.Hour),
	}
	s.routes()
	s.handler = securityHeaders(s.sameOrigin(limitBody(s.mux)))
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.requireUser(s.handleHome))
	s.mux.HandleFunc("GET /style", s.requireAdmin(s.handleStyle))

	// Admin.
	s.mux.HandleFunc("GET /admin", s.requireAdmin(s.handleAdmin))
	s.mux.HandleFunc("GET /admin/group", s.requireAdmin(s.handleGroup))
	s.mux.HandleFunc("POST /admin/group", s.requireAdmin(s.handleGroupPost))
	s.mux.HandleFunc("GET /admin/invites", s.requireAdmin(s.handleInvites))
	s.mux.HandleFunc("POST /admin/invites", s.requireAdmin(s.handleInviteCreate))
	s.mux.HandleFunc("POST /admin/invites/{id}/disable", s.requireAdmin(s.handleInviteDisable))
	s.mux.HandleFunc("GET /admin/members", s.requireAdmin(s.handleMembers))
	s.mux.HandleFunc("POST /admin/members/{id}/disable", s.requireAdmin(s.handleMemberAccess(false)))
	s.mux.HandleFunc("POST /admin/members/{id}/enable", s.requireAdmin(s.handleMemberAccess(true)))

	// Sign-in flow.
	s.mux.HandleFunc("GET /auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /auth/callback", s.handleCallback)
	s.mux.HandleFunc("POST /auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /join/{token}", s.handleJoin)
	// Registered only under the bypass, which config.Load refuses to enable
	// anywhere but http://localhost, so this cannot exist in production.
	if s.cfg.DevBypass {
		s.mux.HandleFunc("POST /auth/dev", s.localOnly(s.handleDevLogin))
	}

	// Unauthenticated. Liveness plus the build, so a deploy can be confirmed
	// from outside without signing in. Fails if the database has gone away,
	// so the container healthcheck notices a lost volume.
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err := s.db.Ping(); err != nil {
			log.Printf("healthz: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("database unavailable\n"))
			return
		}
		_, _ = w.Write([]byte("ok " + s.cfg.Version + "\n"))
	})
	s.mux.Handle("GET /static/", s.staticHandler())

	// Everything else is a styled 404 rather than the mux's plain-text one.
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { s.notFound(w) })
}

// page is the data every template receives.
type page struct {
	Title   string // suffix in the browser tab; empty on the home page
	Version string
	Message string
	Error   string
	User    *store.User    // nil when signed out
	Group   store.Settings // header: group name and timeslot
	Tab     string         // current admin tab

	// Admin forms.
	Form    store.Settings
	Zones   []string
	Saved   bool
	Invites []inviteRow
	Members []memberRow

	DevBypass bool
	Local     bool   // a test copy on this machine: the header says so
	Email     string // the account that was refused, on the denied page
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
		// same-origin, not no-referrer: under no-referrer the browser sends
		// "Origin: null" even on our own form posts (Fetch spec, "append a
		// request Origin header"), and sameOrigin refuses every form (#13).
		// same-origin still sends nothing to other sites.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		// Group members' names and availability, behind a session cookie,
		// passing through a reverse proxy. Nothing in between may cache it.
		// Static assets override this with a revalidating policy.
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// sameOrigin refuses any state-changing request that did not come from one of
// our own pages. SameSite=Lax already keeps the session cookie off cross-site
// POSTs; this is the second lock, and it also covers the signed-out forms.
//
// Browsers send Origin on every POST; Referer is the fallback for the odd
// client that strips it. Both depend on the Referrer-Policy our pages send:
// see securityHeaders.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		ok := origin == s.cfg.BaseURL ||
			(origin == "" && strings.HasPrefix(r.Header.Get("Referer"), s.cfg.BaseURL+"/"))
		if !ok {
			s.refusals.Printf("refused %s %s from origin %q", r.Method, r.URL.Path, origin)
			s.render(w, http.StatusForbidden, "message.html", page{Title: "Not allowed", Message: "That request didn't come from this site."})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// limitBody caps request bodies and parses forms up front, so an oversized
// body is refused here rather than silently read as an empty form by
// FormValue further in.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		if err := r.ParseForm(); err != nil {
			status := http.StatusBadRequest
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				status = http.StatusRequestEntityTooLarge
			}
			http.Error(w, http.StatusText(status), status)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// render writes a page with its status. Taking the status as a parameter keeps
// WriteHeader in one place, after the content type is set.
func (s *Server) render(w http.ResponseWriter, status int, name string, p page) {
	p.Version = s.cfg.Version
	p.DevBypass = s.cfg.DevBypass
	p.Local = s.cfg.Local()
	p.Group = s.groupSettings()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.tpl.ExecuteTemplate(w, name, p); err != nil {
		// Status and headers are already on the wire, so this cannot become a
		// 500. Log it; the client gets a truncated page.
		log.Printf("render %s: %v", name, err)
	}
}

// fail logs the real cause and shows the visitor a generic message.
func (s *Server) fail(w http.ResponseWriter, msg string, err error) {
	log.Printf("error: %s: %v", msg, err)
	s.render(w, http.StatusInternalServerError, "message.html", page{Title: "Something went wrong", Message: msg})
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
// Only files known at startup are served: no directory listings.
func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(staticFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag, ok := s.etags[r.URL.Path]
		if !ok {
			s.notFound(w)
			return
		}
		w.Header().Set("ETag", tag)
		// Revalidate every time rather than trusting a max-age: a stale
		// stylesheet surviving a redeploy looks like a rendering bug and is
		// very hard to recognise as a caching one.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}

func (s *Server) notFound(w http.ResponseWriter) {
	s.render(w, http.StatusNotFound, "message.html", page{Title: "Not found", Message: "There's nothing here."})
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.render(w, http.StatusOK, "home.html", page{User: u})
}

// handleStyle shows the palette, type and icons on a real device. See
// docs/ui.md.
func (s *Server) handleStyle(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.render(w, http.StatusOK, "style.html", page{Title: "Style", Tab: "style", User: u})
}
