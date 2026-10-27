// Package config loads and validates runtime configuration from the environment.
//
// Only deployment facts live here. Group settings (name, timeslot, timezone)
// are edited by the admin and stored in the database. See agents.md > Auth.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
)

// Version is the release this build implements, and Commit the git revision it
// was built from. Both are set at build time with -ldflags
// "-X .../internal/config.Version=2026.1 -X .../internal/config.Commit=$(git rev-parse HEAD)".
//
// Commit has to be injected rather than read from the repository, because the
// Docker build context excludes .git. For a plain local `go build` it is empty
// and VersionString falls back to the revision the toolchain embeds.
var (
	Version = "dev"
	Commit  = ""
)

// shortCommit is how many hex characters of the revision to show.
const shortCommit = 8

// dirtyMark is appended to the revision when the build came from a working
// tree with uncommitted changes. Without it the footer names a commit whose
// contents are not what is actually running.
const dirtyMark = "+"

// VersionString is what the footer and /healthz show: "2026.1 build a1b2c3d4",
// "2026.1 build a1b2c3d4+" when built from a dirty tree, or just "2026.1" when
// the build does not know its commit.
func VersionString() string {
	c := Commit
	if c == "" {
		c = vcsRevision()
	}
	return formatVersion(Version, c)
}

// formatVersion truncates the revision to shortCommit characters, preserving
// any dirty marker, which would otherwise be cut off by the truncation.
func formatVersion(version, commit string) string {
	dirty := strings.HasSuffix(commit, dirtyMark)
	commit = strings.TrimSuffix(commit, dirtyMark)
	if len(commit) > shortCommit {
		commit = commit[:shortCommit]
	}
	if commit == "" {
		return version
	}
	if dirty {
		commit += dirtyMark
	}
	return version + " build " + commit
}

// vcsRevision reads the revision the Go toolchain stamps into the binary,
// marking it dirty if the toolchain saw uncommitted changes. This works for a
// local `go build` in a git checkout and yields nothing otherwise.
func vcsRevision() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev string
	var modified bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev != "" && modified {
		rev += dirtyMark
	}
	return rev
}

type Config struct {
	Listen  string
	BaseURL string // public origin, e.g. https://assemble.example.com. No trailing slash.
	Version string

	// DBPath is the SQLite file. The default is relative so a local build
	// works with no configuration; in the container it is an absolute path on
	// a mounted volume, because the database must never live in the image.
	DBPath string

	GoogleClientID     string
	GoogleClientSecret string

	// AdminEmail bootstraps the first admin: the Google account that signs in
	// with this verified address becomes admin and is bound to its sub. The
	// only place an email identifies anyone. See agents.md > Auth.
	AdminEmail string

	// DevBypass replaces Google with a local form that signs in as any email
	// typed into it, so join and disable flows can be tried with several
	// "people". Refuses to activate unless BaseURL is http on localhost.
	DevBypass bool
}

// RedirectURL is the OAuth callback registered in the Google Cloud console.
func (c Config) RedirectURL() string { return c.BaseURL + "/auth/callback" }

func Load() (Config, error) {
	c := Config{
		Listen:             envOr("ASSEMBLE_LISTEN", ":8080"),
		BaseURL:            strings.TrimRight(os.Getenv("ASSEMBLE_BASE_URL"), "/"),
		Version:            VersionString(),
		DBPath:             envOr("ASSEMBLE_DB_PATH", "data/assemble.db"),
		GoogleClientID:     os.Getenv("ASSEMBLE_GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("ASSEMBLE_GOOGLE_CLIENT_SECRET"),
		AdminEmail:         strings.TrimSpace(os.Getenv("ASSEMBLE_ADMIN_EMAIL")),
		DevBypass:          os.Getenv("ASSEMBLE_DEV_BYPASS") == "1",
	}

	var missing []string
	if c.BaseURL == "" {
		missing = append(missing, "ASSEMBLE_BASE_URL")
	}
	if c.AdminEmail == "" {
		missing = append(missing, "ASSEMBLE_ADMIN_EMAIL")
	}
	if !c.DevBypass {
		if c.GoogleClientID == "" {
			missing = append(missing, "ASSEMBLE_GOOGLE_CLIENT_ID")
		}
		if c.GoogleClientSecret == "" {
			missing = append(missing, "ASSEMBLE_GOOGLE_CLIENT_SECRET")
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment: %s", strings.Join(missing, ", "))
	}

	u, err := url.Parse(c.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Config{}, errors.New("ASSEMBLE_BASE_URL must include the scheme, e.g. https://assemble.example.com")
	}
	if c.DevBypass && (u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1")) {
		return Config{}, errors.New("ASSEMBLE_DEV_BYPASS only runs against http://localhost or http://127.0.0.1")
	}
	return c, nil
}

// Secure reports whether cookies should carry the Secure attribute. Tied to the
// public origin rather than the request, because TLS terminates at the proxy and
// the request arriving here is plain http.
func (c Config) Secure() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
