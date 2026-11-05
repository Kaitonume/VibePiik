package nativeui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/TNTcraftHIM/Piik/internal/app/buildinfo"
	"golang.org/x/mod/semver"
)

// releaseNotice is the web ReleaseUpdateNotice (lib/release-update.ts): a
// published Piik release that differs from the running App build.
type releaseNotice struct {
	Kind       string // update-available, major-update, different-build, official-release
	Version    string
	URL        string // download (package asset when there is one)
	ReleaseURL string // release notes
	Body       string // release notes (Markdown): the changelog
	Important  bool   // <!-- importance: important --> in the notes
	Digest     string // SHA-256 of the package (hex), when GitHub gives it
	Package    bool   // URL is the Windows package itself (it can be installed)
}

// releaseRepo is the GitHub repository ("owner/name") whose releases VibePiik
// checks. Empty: no check. It must never be the official Piik repository:
// updating to an official build would drop every VibePiik feature.
const releaseRepo = "Kaitonume/VibePiik"

// vibeVersion is VibePiik's own release (the App still reports Piik v2.5.3 to
// rooms and sites). VibePiik releases are tagged vMAJOR.MINOR.PATCH.
const vibeVersion = "v1.1.0"

var (
	releaseAPI       = "https://api.github.com/repos/" + releaseRepo + "/releases/latest"
	releaseMirrorAPI = "" // no AtomGit mirror for VibePiik
	releasePage      = "https://github.com/" + releaseRepo + "/releases/tag/"
	releaseMirror    = ""
)

var (
	revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	tagPattern      = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	digestPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	mirrorSource    = regexp.MustCompile(`(?m)^<!-- piik-source: ([0-9a-f]{40}) -->\r?$`)
)

func normalizeRevision(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if revisionPattern.MatchString(value) {
		return value
	}
	return ""
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	if semver.IsValid(value) && semver.Canonical(value) == value {
		return value
	}
	return ""
}

type releaseAsset struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	State    string `json:"state"`
	Type     string `json:"type"`
	Download string `json:"browser_download_url"`
	Digest   string `json:"digest"` // "sha256:<hex>"
}

type publishedRelease struct {
	version, revision, url, releaseURL string
	body, digest                       string
	isPackage                          bool
}

// releaseTestFeed: automated tests serve a fake release from this PC
// (VIBEPIIK_UPDATE_FEED); its addresses are not GitHub's.
var releaseTestFeed = false

// packageURL picks this platform's package from the release assets, like
// packageDownloadURL; otherwise the release page.
func packageURL(release publishedRelease, assets []releaseAsset, target string, mirror bool) string {
	if target == "" || len(assets) == 0 {
		return release.url
	}
	extension := "zip"
	if target == "android-universal" {
		extension = "apk"
	}
	names := []string{"VibePiik-" + target + "." + extension, "piik-app-" + target + "." + extension}
	if release.revision != "" && extension == "zip" {
		names = append(names, "piik-app-"+target+"-"+release.revision[:7]+".zip")
	}
	for _, name := range names {
		var matches []releaseAsset
		for _, asset := range assets {
			if asset.Name == name {
				matches = append(matches, asset)
			}
		}
		if len(matches) == 0 {
			continue
		}
		asset := matches[0]
		expected := strings.Replace(release.releaseURL, "/releases/tag/", "/releases/download/", 1) + "/" + name
		if mirror {
			expected = releaseMirror + "/download/" + release.version + "/" + name
		}
		ok := len(matches) == 1 && (asset.Download == expected || releaseTestFeed)
		if mirror {
			ok = ok && asset.Type == "attach"
		} else {
			ok = ok && asset.Size > 0 && asset.State == "uploaded"
		}
		if ok {
			return asset.Download
		}
		return release.url
	}
	return release.url
}

type githubRelease struct {
	Tag        string         `json:"tag_name"`
	HTML       string         `json:"html_url"`
	Commitish  string         `json:"target_commitish"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Body       string         `json:"body"`
	Assets     []releaseAsset `json:"assets"`
}

func parseRelease(r githubRelease, target string) (publishedRelease, bool) {
	if r.Draft || r.Prerelease || !tagPattern.MatchString(r.Tag) || normalizeVersion(r.Tag) != r.Tag || (r.HTML != releasePage+r.Tag && !releaseTestFeed) {
		return publishedRelease{}, false
	}
	release := publishedRelease{version: r.Tag, revision: normalizeRevision(r.Commitish), url: releasePage + r.Tag, releaseURL: releasePage + r.Tag, body: r.Body}
	release.url = packageURL(release, r.Assets, target, false)
	for _, asset := range r.Assets {
		if asset.Download == release.url && strings.HasSuffix(asset.Name, ".zip") {
			release.isPackage = true
			if digest := strings.ToLower(strings.TrimPrefix(asset.Digest, "sha256:")); digestPattern.MatchString(digest) {
				release.digest = digest
			}
		}
	}
	return release, true
}

func parseMirrorRelease(r githubRelease, target string) (publishedRelease, bool) {
	if r.Prerelease {
		return publishedRelease{}, false
	}
	sources := mirrorSource.FindAllStringSubmatch(r.Body, -1)
	if len(sources) != 1 {
		return publishedRelease{}, false
	}
	release, ok := parseRelease(githubRelease{Tag: r.Tag, HTML: releasePage + r.Tag, Commitish: sources[0][1]}, "")
	if !ok {
		return publishedRelease{}, false
	}
	release.url, release.releaseURL = releaseMirror, releaseMirror
	release.url = packageURL(release, r.Assets, target, true)
	return release, true
}

// noticeFor is releaseUpdateNotice.
func noticeFor(version, revision string, latest publishedRelease) *releaseNotice {
	notice := func(kind string) *releaseNotice {
		return &releaseNotice{Kind: kind, Version: latest.version, URL: latest.url, ReleaseURL: latest.releaseURL,
			Body: latest.body, Important: releaseImportant(latest.body), Digest: latest.digest, Package: latest.isPackage}
	}
	version, revision = normalizeVersion(version), normalizeRevision(revision)
	if version != "" {
		order := semver.Compare(latest.version, version)
		switch {
		case order > 0 && semver.Major(latest.version) != semver.Major(version):
			return notice("major-update")
		case order > 0:
			return notice("update-available")
		case order == 0 && revision != "" && latest.revision != "" && revision != latest.revision:
			return notice("different-build")
		}
		return nil
	}
	if revision != "" && latest.revision != "" && revision != latest.revision {
		return notice("official-release")
	}
	return nil
}

var releaseClient = &http.Client{Timeout: 3 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func fetchJSON(ctx context.Context, url string, mirror bool, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if mirror {
		request.Header.Set("Accept", "application/json")
	} else {
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	request.Header.Set("User-Agent", "VibePiik")
	response, err := releaseClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(out)
}

// checkRelease is checkReleaseUpdate: GitHub first, then the AtomGit mirror.
func checkRelease(ctx context.Context, version, revision, target string) *releaseNotice {
	if normalizeVersion(version) == "" && normalizeRevision(revision) == "" {
		return nil
	}
	var latest githubRelease
	if fetchJSON(ctx, releaseAPI, false, &latest) == nil {
		if release, ok := parseRelease(latest, target); ok {
			return noticeFor(version, revision, release)
		}
	}
	if releaseMirrorAPI == "" {
		return nil
	}
	var best *publishedRelease
	for page := 1; page <= 10; page++ {
		var list []githubRelease
		url := fmt.Sprintf("%s?per_page=100&page=%d&direction=desc", releaseMirrorAPI, page)
		if fetchJSON(ctx, url, true, &list) != nil || len(list) > 100 {
			return nil
		}
		for _, item := range list {
			if candidate, ok := parseMirrorRelease(item, target); ok && (best == nil || semver.Compare(candidate.version, best.version) > 0) {
				candidate := candidate
				best = &candidate
			}
		}
		if len(list) < 100 {
			if best != nil {
				return noticeFor(version, revision, *best)
			}
			return nil
		}
	}
	return nil
}

// releaseCheck runs the check once per App build and keeps the answer.
var releaseCheck struct {
	sync.Mutex
	key     string
	started bool
	notice  *releaseNotice
}

// appRelease returns the known notice and starts the check when needed;
// done is called once a new answer is in.
func appRelease(version, revision string, done func()) *releaseNotice {
	// VibePiik compares its own version with its own releases; the Piik
	// version and revision it is built from do not take part.
	version, revision = vibeVersion, ""
	key := version + "|" + revision
	releaseCheck.Lock()
	defer releaseCheck.Unlock()
	if releaseCheck.key != key {
		releaseCheck.key, releaseCheck.started, releaseCheck.notice = key, false, nil
	}
	if !releaseCheck.started && !releaseCheckOff && releaseRepo != "" {
		releaseCheck.started = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			notice := checkRelease(ctx, version, revision, buildinfo.PackageTarget())
			releaseCheck.Lock()
			if releaseCheck.key == key {
				releaseCheck.notice = notice
			}
			releaseCheck.Unlock()
			if notice != nil && done != nil {
				done()
			}
		}()
	}
	return releaseCheck.notice
}

// releaseView is AppReleaseNotice: download button, AtomGit link and, for a
// major release, the breaking-changes warning.
type releaseView struct {
	download, mirror, notes Button
}

func (v *releaseView) Layout(gtx layout.Context, u *UI, notice *releaseNotice, open func(string)) layout.Dimensions {
	if notice == nil {
		return layout.Dimensions{}
	}
	if v.download.Clicked(gtx) {
		if notice.Package {
			openUpdateModal() // changelog, then install and restart
		} else if open != nil {
			go open(notice.URL)
		}
	}
	if open != nil {
		if v.mirror.Clicked(gtx) {
			go open(releaseMirror)
		}
		if v.notes.Clicked(gtx) {
			go open(notice.ReleaseURL)
		}
	}
	major := notice.Kind == "major-update"
	label := u.T(releaseLabels[notice.Kind]) + " " + notice.Version
	caption := label
	if major || notice.Kind == "update-available" {
		caption = u.T("client.update.download", "version", notice.Version)
		if notice.Package {
			caption = u.T("native.update.button", "version", notice.Version)
		}
	}
	icon, tip := "arrowUp", label
	if major {
		icon, tip = "alert", u.T("client.update.majorHint")
	}
	if u.Visual() {
		caption = notice.Version
	}
	mirror := u.T("client.update.downloads") + " ↗"
	if u.Visual() {
		mirror = "AtomGit ↗"
	}
	hasMirror := releaseMirror != ""
	gtx.Constraints.Min.X = 0
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					tone := toneDefault
					if notice.Important {
						tone = tonePrimary
					}
					return v.download.Layout(gtx, u, ButtonStyle{Comic: "update-available", Icon: icon, Caption: caption, Size: sizeCompact, Tip: tip, Tone: tone})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasMirror {
						return layout.Dimensions{}
					}
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return v.mirror.Layout(gtx, u, ButtonStyle{Caption: mirror, Size: sizeDense, Tone: toneQuiet})
					})
				}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !major {
				return layout.Dimensions{}
			}
			caption := u.T("client.update.majorHint") + " ↗"
			if u.Visual() {
				caption = ""
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return v.notes.Layout(gtx, u, ButtonStyle{Icon: "alert", Caption: caption, Size: sizeDense, Tone: toneQuiet})
			})
		}))
}

// projectPage is where Help's GitHub button goes: VibePiik's repository once
// it is published, until then the Piik project page (not its downloads).
func projectPage() string {
	if releaseRepo != "" {
		return "https://github.com/" + releaseRepo
	}
	return "https://github.com/TNTcraftHIM/Piik"
}

// releaseCheckOff keeps tests off the network.
var releaseCheckOff = false

var releaseLabels = map[string]string{
	"update-available": "client.update.available",
	"major-update":     "client.update.major",
	"different-build":  "client.update.differentBuild",
	"official-release": "client.update.official",
}

func init() {
	nativeCopy_en["client.update.available"] = "A newer VibePiik release is available"
	nativeCopy_en["client.update.major"] = "New major VibePiik release"
	nativeCopy_en["client.update.differentBuild"] = "This build differs from the VibePiik release"
	nativeCopy_en["client.update.official"] = "A VibePiik release is available"
	nativeCopy_zh["client.update.available"] = "发现新的 VibePiik 版本"
	nativeCopy_zh["client.update.major"] = "发现新的 VibePiik 大版本"
	nativeCopy_zh["client.update.differentBuild"] = "当前构建与 VibePiik 发行版不同"
	nativeCopy_zh["client.update.official"] = "可获取 VibePiik 发行版"
	nativeCopy_it["client.update.available"] = "È disponibile una nuova versione di VibePiik"
	nativeCopy_it["client.update.major"] = "Nuova versione principale di VibePiik"
	nativeCopy_it["client.update.differentBuild"] = "Questa build è diversa dalla versione di VibePiik"
	nativeCopy_it["client.update.official"] = "È disponibile una versione di VibePiik"
}
