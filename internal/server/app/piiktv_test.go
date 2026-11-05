package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The send endpoint is for the Host's own page only: never a tunnel visitor or a LAN peer.
func TestPiikTVLocalRequest(t *testing.T) {
	local := httptest.NewRequest("POST", "http://127.0.0.1:8787/api/piik-tv/send", nil)
	local.RemoteAddr = "127.0.0.1:50000"
	if !piikTVLocalRequest(local) {
		t.Fatal("loopback page refused")
	}
	tunnel := httptest.NewRequest("POST", "http://x.trycloudflare.com/api/piik-tv/send", nil)
	tunnel.RemoteAddr = "127.0.0.1:50001"
	tunnel.Header.Set("Cf-Connecting-Ip", "203.0.113.9")
	if piikTVLocalRequest(tunnel) {
		t.Fatal("tunnel request accepted")
	}
	lan := httptest.NewRequest("POST", "http://192.168.0.5:8787/api/piik-tv/send", nil)
	lan.RemoteAddr = "192.168.0.8:40000"
	if piikTVLocalRequest(lan) {
		t.Fatal("LAN request accepted")
	}
}

// The Host page reads the language of the Piik TV texts; visitors cannot.
func TestPiikTVLanguage(t *testing.T) {
	server := &Server{}
	local := httptest.NewRequest("GET", "http://127.0.0.1:8787/api/piik-tv/language", nil)
	local.RemoteAddr = "127.0.0.1:50000"
	response := httptest.NewRecorder()
	server.handlePiikTVLanguage(response, local)
	var body struct {
		Language string `json:"language"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil ||
		(body.Language != "it" && body.Language != "en") {
		t.Fatalf("language = %d %s", response.Code, response.Body.String())
	}
	tunnel := httptest.NewRequest("GET", "http://x.trycloudflare.com/api/piik-tv/language", nil)
	tunnel.RemoteAddr = "127.0.0.1:50001"
	tunnel.Header.Set("Cf-Connecting-Ip", "203.0.113.9")
	response = httptest.NewRecorder()
	server.handlePiikTVLanguage(response, tunnel)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tunnel language = %d", response.Code)
	}
	post := httptest.NewRequest("POST", "http://127.0.0.1:8787/api/piik-tv/language", nil)
	post.RemoteAddr = "127.0.0.1:50002"
	response = httptest.NewRecorder()
	server.handlePiikTVLanguage(response, post)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST language = %d", response.Code)
	}
}
