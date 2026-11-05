// Package appwindow shows the App pages in VibePiik's own window instead of the
// system browser: one isolated WebView2 window with no tabs, extensions or
// browsing profile. Closing it quits the App. Links that leave the App open in
// the system browser.
package appwindow

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnavailable means the window cannot be shown (no WebView2 Runtime).
var ErrUnavailable = errors.New("the VibePiik window is unavailable: the WebView2 Runtime is not installed")

const (
	messagePrefix = "vibepiik:"
	defaultTitle  = "VibePiik"
	// Logical size (96 DPI) of a new window; scaled to the system DPI.
	defaultWidth, defaultHeight = 1200, 880
	minimumWidth, minimumHeight = 720, 560
)

// initScript runs in every page before its own scripts. Links and popups that
// leave the App go to the system browser; the page title follows the window.
const initScript = `(() => {
  if (window.__vibepiikWindow) return;
  window.__vibepiikWindow = true;
  const post = (kind, value) => {
    try { window.chrome.webview.postMessage("vibepiik:" + kind + ":" + value); } catch (_) {}
  };
  const parse = (raw) => {
    try {
      const url = new URL(raw, location.href);
      return url.protocol === "http:" || url.protocol === "https:" ? url : null;
    } catch (_) { return null; }
  };
  const local = (url) => url.hostname === "127.0.0.1" || url.hostname === "localhost" || url.hostname === "[::1]";
  document.addEventListener("click", (event) => {
    if (event.defaultPrevented || event.button !== 0) return;
    const anchor = event.target instanceof Element ? event.target.closest("a[href]") : null;
    if (!anchor) return;
    const url = parse(anchor.href);
    if (!url || (anchor.target !== "_blank" && local(url))) return;
    event.preventDefault();
    post("open", url.href);
  }, true);
  const nativeOpen = window.open;
  window.open = function (raw, target, features) {
    const url = raw ? parse(String(raw)) : null;
    if (url && !local(url)) { post("open", url.href); return null; }
    return nativeOpen.call(window, raw, target, features);
  };
  let title = null;
  const sendTitle = () => { if (document.title !== title) { title = document.title; post("title", title); } };
  document.addEventListener("DOMContentLoaded", () => {
    sendTitle();
    new MutationObserver(sendTitle).observe(document.head, { subtree: true, childList: true, characterData: true });
  });
})();`

type message struct {
	open  string
	title string
}

// parseMessage accepts only the two messages sent by initScript. An open
// request must be an absolute http(s) URL.
func parseMessage(raw string) (message, bool) {
	body, found := strings.CutPrefix(raw, messagePrefix)
	if !found {
		return message{}, false
	}
	kind, value, found := strings.Cut(body, ":")
	if !found {
		return message{}, false
	}
	switch kind {
	case "open":
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return message{}, false
		}
		return message{open: parsed.String()}, true
	case "title":
		return message{title: windowTitle(value)}, true
	}
	return message{}, false
}

// windowTitle names the window after the page: "Piik | 6049 · Sharing" becomes
// "VibePiik | 6049 · Sharing".
func windowTitle(page string) string {
	page = strings.TrimSpace(page)
	if page == "" {
		return defaultTitle
	}
	if rest, found := strings.CutPrefix(page, "Piik"); found {
		return defaultTitle + rest
	}
	return defaultTitle + " | " + page
}

// placement is the window's restored rectangle in screen pixels.
type placement struct {
	Left, Top, Right, Bottom int32
	Maximized                bool
}

func (p placement) valid() bool {
	return p.Right-p.Left >= minimumWidth/2 && p.Bottom-p.Top >= minimumHeight/2 &&
		p.Right-p.Left <= 32768 && p.Bottom-p.Top <= 32768
}

func dataDirectory() string {
	base, err := os.UserCacheDir() // %LOCALAPPDATA% on Windows
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "VibePiik")
}

func loadPlacement(path string) (placement, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return placement{}, false
	}
	var value placement
	if json.Unmarshal(data, &value) != nil || !value.valid() {
		return placement{}, false
	}
	return value, true
}

func savePlacement(path string, value placement) {
	if !value.valid() {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o600)
}
