package pkgmeta

import (
	"testing"

	"github.com/quant-on-quest/johnnybt-pypi/internal/testutil"
)

func TestParseMetadata(t *testing.T) {
	raw := "Metadata-Version: 2.1\nName: johnnybt-demo\nVersion: 0.1.0\nSummary: Demo package\nRequires-Python: >=3.10\nDescription-Content-Type: text/markdown\n\n# Title\n\nBody line 1\nBody line 2\n"
	m, err := ParseMetadata([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "johnnybt-demo" || m.Version != "0.1.0" || m.Summary != "Demo package" {
		t.Errorf("basic fields wrong: %+v", m)
	}
	if m.RequiresPython != ">=3.10" {
		t.Errorf("RequiresPython = %q", m.RequiresPython)
	}
	if m.DescriptionContentType != "text/markdown" {
		t.Errorf("DescriptionContentType = %q", m.DescriptionContentType)
	}
	if m.Description != "# Title\n\nBody line 1\nBody line 2" {
		t.Errorf("Description = %q", m.Description)
	}
}

func TestParseMetadataHeaderDescription(t *testing.T) {
	m, err := ParseMetadata([]byte("Name: x\nVersion: 1\nDescription: inline text\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Description != "inline text" {
		t.Errorf("Description = %q", m.Description)
	}
}

func TestParseMetadataNoBody(t *testing.T) {
	m, err := ParseMetadata([]byte("Name: x\nVersion: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "x" || m.Description != "" {
		t.Errorf("got %+v", m)
	}
}

func TestWheelMetadata(t *testing.T) {
	dir := t.TempDir()
	md := testutil.Metadata("johnnybt-demo", "0.1.0", map[string]string{"Summary": "hi"}, "")
	p := testutil.WriteFile(t, dir, "johnnybt_demo-0.1.0-py3-none-any.whl", testutil.WheelBytes(t, "johnnybt_demo", "0.1.0", md))
	raw, err := WheelMetadata(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != md {
		t.Errorf("metadata mismatch:\n%s", raw)
	}
}

func TestWheelMetadataMissing(t *testing.T) {
	dir := t.TempDir()
	p := testutil.WriteFile(t, dir, "bad.whl", []byte("not a zip"))
	if _, err := WheelMetadata(p); err == nil {
		t.Error("expected error for non-zip wheel")
	}
}

func TestSdistMetadata(t *testing.T) {
	dir := t.TempDir()
	md := testutil.Metadata("johnnybt-demo", "0.1.0", nil, "readme")
	p := testutil.WriteFile(t, dir, "johnnybt_demo-0.1.0.tar.gz", testutil.SdistBytes(t, "johnnybt_demo", "0.1.0", md))
	raw, err := SdistMetadata(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != md {
		t.Errorf("metadata mismatch (decoy PKG-INFO picked?):\n%s", raw)
	}
}
