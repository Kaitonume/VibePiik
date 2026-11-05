package nativeui

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	releaseCheckOff = true // no GitHub requests from tests
	screenDrag = false     // drags come from the simulated pointer
	os.Exit(m.Run())
}

func TestReleaseNotice(t *testing.T) {
	if strings.Contains(strings.ToLower(releaseRepo), "tntcrafthim/piik") {
		t.Fatal("VibePiik must not offer official Piik releases: updating would drop its features")
	}
	sha := strings.Repeat("a", 40)
	other := strings.Repeat("b", 40)
	latest := publishedRelease{version: "v2.6.0", revision: sha, url: "u", releaseURL: "r"}
	cases := []struct {
		version, revision, kind string
	}{
		{"v2.5.3", other, "update-available"},
		{"v1.9.0", "", "major-update"},
		{"v2.6.0", other, "different-build"},
		{"v2.6.0", sha, ""},
		{"v2.7.0", "", ""},
		{"development", other, "official-release"},
		{"development", "development", ""},
	}
	for _, c := range cases {
		notice := noticeFor(c.version, c.revision, latest)
		kind := ""
		if notice != nil {
			kind = notice.Kind
		}
		if kind != c.kind {
			t.Errorf("%s/%s: got %q, want %q", c.version, c.revision, kind, c.kind)
		}
	}
	release, ok := parseRelease(githubRelease{Tag: "v2.6.0", HTML: releasePage + "v2.6.0", Commitish: sha,
		Assets: []releaseAsset{{Name: "piik-app-windows-amd64.zip", Size: 10, State: "uploaded",
			Download: strings.Replace(releasePage, "/releases/tag/", "/releases/download/", 1) + "v2.6.0/piik-app-windows-amd64.zip"}}}, "windows-amd64")
	if !ok || !strings.HasSuffix(release.url, "piik-app-windows-amd64.zip") {
		t.Fatalf("package URL: %v %+v", ok, release)
	}
	if _, ok := parseRelease(githubRelease{Tag: "v2.6.0", HTML: "https://evil.example/v2.6.0"}, ""); ok {
		t.Fatal("a release page elsewhere must be refused")
	}
	mirror, ok := parseMirrorRelease(githubRelease{Tag: "v2.6.0", Body: "notes\n<!-- piik-source: " + sha + " -->\n"}, "")
	if !ok || mirror.revision != sha || mirror.url != releaseMirror {
		t.Fatalf("mirror: %v %+v", ok, mirror)
	}
}
