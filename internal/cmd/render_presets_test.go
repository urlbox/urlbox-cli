package cmd

// White-box tests for applyPreset and the canonical preset map. Pinning
// these values keeps a silent edit from changing the defaults that
// downstream automation depends on.

import (
	"reflect"
	"strings"
	"testing"
)

func TestApplyPreset_Mobile_PinnedValues(t *testing.T) {
	got := applyPreset("mobile")
	if got["width"] != 375 {
		t.Errorf("width=%v, want 375", got["width"])
	}
	if got["height"] != 812 {
		t.Errorf("height=%v, want 812", got["height"])
	}
	ua, _ := got["user_agent"].(string)
	if ua == "" {
		t.Fatalf("user_agent empty")
	}
	// Pin the prefix so a silent UA edit fails this test.
	if !strings.HasPrefix(ua, "Mozilla/5.0 (iPhone; CPU iPhone") {
		t.Errorf("user_agent prefix changed: %q", ua)
	}
}

func TestApplyPreset_Desktop(t *testing.T) {
	got := applyPreset("desktop")
	want := map[string]any{"width": 1920, "height": 1080}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%v, want %v", got, want)
	}
}

func TestApplyPreset_PDFA4(t *testing.T) {
	got := applyPreset("pdf-a4")
	if got["format"] != "pdf" {
		t.Errorf("format=%v", got["format"])
	}
	if got["pdf_page_size"] != "A4" {
		t.Errorf("pdf_page_size=%v", got["pdf_page_size"])
	}
}

func TestApplyPreset_Unknown_ReturnsNil(t *testing.T) {
	if got := applyPreset("nonsense"); got != nil {
		t.Errorf("unknown preset returned %v, want nil", got)
	}
}

func TestApplyPreset_ReturnsCopyNotReference(t *testing.T) {
	a := applyPreset("desktop")
	a["width"] = 9999
	b := applyPreset("desktop")
	if b["width"] == 9999 {
		t.Errorf("applyPreset returned a shared reference (mutation leaked across calls)")
	}
}

func TestAvailablePresetNames_StableOrder(t *testing.T) {
	got := availablePresetNames()
	want := "article, desktop, mobile, pdf-a4"
	if got != want {
		t.Errorf("got=%q, want %q (alphabetical order, comma-separated)", got, want)
	}
}

func TestApplyPreset_Article_PinnedValues(t *testing.T) {
	got := applyPreset("article")
	if got == nil {
		t.Fatalf("applyPreset(\"article\") returned nil; expected the preset to exist")
	}
	if got["block_ads"] != true {
		t.Errorf("block_ads=%v, want true", got["block_ads"])
	}
	if got["retina"] != true {
		t.Errorf("retina=%v, want true", got["retina"])
	}
	if got["wait_until"] != "mostrequestsfinished" {
		t.Errorf("wait_until=%v, want mostrequestsfinished", got["wait_until"])
	}
	if got["full_page"] != false {
		t.Errorf("full_page=%v, want false", got["full_page"])
	}
}

func TestAvailablePresetNames_IncludesArticle(t *testing.T) {
	got := availablePresetNames()
	if !strings.Contains(got, "article") {
		t.Errorf("availablePresetNames()=%q, want it to contain \"article\"", got)
	}
}
