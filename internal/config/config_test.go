package config

import "testing"

func TestFormatVersion(t *testing.T) {
	for _, tc := range []struct{ version, commit, want string }{
		{"2026.1", "", "2026.1"},
		{"2026.1", "a1b2c3d4e5f6", "2026.1 build a1b2c3d4"},
		{"2026.1", "a1b2c3d4e5f6+", "2026.1 build a1b2c3d4+"},
		{"dev", "abc", "dev build abc"},
	} {
		if got := formatVersion(tc.version, tc.commit); got != tc.want {
			t.Errorf("formatVersion(%q, %q) = %q, want %q", tc.version, tc.commit, got, tc.want)
		}
	}
}

// setEnv sets a complete, valid production environment, which each test then
// breaks in one place.
func setEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ASSEMBLE_BASE_URL", "https://assemble.example.com")
	t.Setenv("ASSEMBLE_GOOGLE_CLIENT_ID", "id")
	t.Setenv("ASSEMBLE_GOOGLE_CLIENT_SECRET", "secret")
	t.Setenv("ASSEMBLE_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("ASSEMBLE_DEV_BYPASS", "")
}

func TestLoadValid(t *testing.T) {
	setEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RedirectURL() != "https://assemble.example.com/auth/callback" {
		t.Errorf("RedirectURL = %q", c.RedirectURL())
	}
}

func TestLoadRequires(t *testing.T) {
	for _, k := range []string{"ASSEMBLE_BASE_URL", "ASSEMBLE_ADMIN_EMAIL", "ASSEMBLE_GOOGLE_CLIENT_ID", "ASSEMBLE_GOOGLE_CLIENT_SECRET"} {
		setEnv(t)
		t.Setenv(k, "")
		if _, err := Load(); err == nil {
			t.Errorf("Load without %s succeeded", k)
		}
	}
}

func TestLoadRequiresScheme(t *testing.T) {
	setEnv(t)
	t.Setenv("ASSEMBLE_BASE_URL", "assemble.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("Load with a schemeless base URL succeeded")
	}
}

func TestSecureFollowsBaseURL(t *testing.T) {
	setEnv(t)
	t.Setenv("ASSEMBLE_BASE_URL", "https://assemble.example.com/")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Secure() {
		t.Error("https base URL: Secure() = false")
	}
	if c.BaseURL != "https://assemble.example.com" {
		t.Errorf("trailing slash kept: %q", c.BaseURL)
	}

	t.Setenv("ASSEMBLE_BASE_URL", "http://localhost:8138")
	if c, _ := Load(); c.Secure() {
		t.Error("http base URL: Secure() = true")
	}
}

// The bypass is the one setting that would hand anyone the admin account, so
// it must be impossible to switch on anywhere but a developer's own machine.
func TestDevBypassOnlyOnLocalhost(t *testing.T) {
	for _, tc := range []struct {
		base string
		ok   bool
	}{
		{"http://localhost:8138", true},
		{"http://127.0.0.1:8138", true},
		{"https://localhost:8138", false},
		{"https://assemble.example.com", false},
		{"http://assemble.example.com", false},
		{"http://192.168.1.20:8138", false},
		{"http://localhost.example.com", false},
	} {
		setEnv(t)
		t.Setenv("ASSEMBLE_BASE_URL", tc.base)
		t.Setenv("ASSEMBLE_DEV_BYPASS", "1")
		_, err := Load()
		if (err == nil) != tc.ok {
			t.Errorf("bypass with %s: err = %v, want ok=%v", tc.base, err, tc.ok)
		}
	}
}

func TestDevBypassNeedsNoGoogle(t *testing.T) {
	setEnv(t)
	t.Setenv("ASSEMBLE_BASE_URL", "http://localhost:8138")
	t.Setenv("ASSEMBLE_DEV_BYPASS", "1")
	t.Setenv("ASSEMBLE_GOOGLE_CLIENT_ID", "")
	t.Setenv("ASSEMBLE_GOOGLE_CLIENT_SECRET", "")
	if _, err := Load(); err != nil {
		t.Fatalf("bypass without Google: %v", err)
	}
}
