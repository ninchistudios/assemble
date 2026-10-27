package config

import "testing"

func TestFormatVersion(t *testing.T) {
	for _, tc := range []struct{ version, commit, want string }{
		{"2026.1", "", "2026.1"},
		{"2026.1", "a1b2c3d4e5f6", "2026.1 a1b2c3d"},
		{"2026.1", "a1b2c3d4e5f6+", "2026.1 a1b2c3d+"},
		{"dev", "abc", "dev abc"},
	} {
		if got := formatVersion(tc.version, tc.commit); got != tc.want {
			t.Errorf("formatVersion(%q, %q) = %q, want %q", tc.version, tc.commit, got, tc.want)
		}
	}
}

func TestLoadRequiresBaseURL(t *testing.T) {
	t.Setenv("ASSEMBLE_BASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load with no base URL succeeded")
	}
}

func TestLoadRequiresScheme(t *testing.T) {
	t.Setenv("ASSEMBLE_BASE_URL", "assemble.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("Load with a schemeless base URL succeeded")
	}
}

func TestSecureFollowsBaseURL(t *testing.T) {
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
