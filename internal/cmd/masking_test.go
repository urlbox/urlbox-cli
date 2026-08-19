package cmd

import (
	"strings"
	"testing"
)

func TestMaskProxyURLMasksOnlyPassword(t *testing.T) {
	got := maskProxyURL("http://user:hunter2@proxy.example.com:8080", false)
	want := "http://user:****@proxy.example.com:8080"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMaskProxyURLRevealReturnsRaw(t *testing.T) {
	raw := "http://user:hunter2@proxy.example.com:8080"
	if got := maskProxyURL(raw, true); got != raw {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestMaskProxyURLNoPasswordUntouched(t *testing.T) {
	raw := "http://proxy.example.com:8080"
	if got := maskProxyURL(raw, false); got != raw {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestMaskProxyURLSchemelessWithAtFullMasks(t *testing.T) {
	got := maskProxyURL("user:hunter2@proxy.example.com:8080", false)
	if got == "user:hunter2@proxy.example.com:8080" {
		t.Fatalf("schemeless credential string must not pass through unmasked")
	}
}

func TestMaskProxyURLHidesPassword(t *testing.T) {
	got := maskProxyURL("http://user:hunter2@proxy.example.com:8080", false)
	if strings.Contains(got, "hunter2") {
		t.Fatalf("password leaked: %q", got)
	}
	if !strings.Contains(got, "proxy.example.com") {
		t.Fatalf("host missing: %q", got)
	}
}

func TestMaskProxyURLMasksSchemelessCredentials(t *testing.T) {
	got := maskProxyURL("user:hunter2@proxy.example.com:8080", false)
	if strings.Contains(got, "hunter2") {
		t.Fatalf("password leaked for scheme-less url: %q", got)
	}
}

func TestMaskProxyURLRevealAndNoAuthPassThrough(t *testing.T) {
	if got := maskProxyURL("http://user:hunter2@p.example.com", true); !strings.Contains(got, "hunter2") {
		t.Fatalf("reveal should keep password: %q", got)
	}
	if got := maskProxyURL("http://plain.example.com:3128", false); got != "http://plain.example.com:3128" {
		t.Fatalf("auth-less url must pass through: %q", got)
	}
}
