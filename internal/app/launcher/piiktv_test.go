package launcher

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TNTcraftHIM/Piik/internal/piiktv"
)

// The launcher page sends any room link it is given, also before its own room exists.
func TestLauncherSendsChosenLinkToPiikTV(t *testing.T) {
	var sent []string
	previous := sendToPiikTV
	sendToPiikTV = func(_ context.Context, link string) []piiktv.Target {
		sent = append(sent, link)
		if strings.Contains(link, "/r/2222") {
			return []piiktv.Target{}
		}
		return []piiktv.Target{{Address: "192.168.0.5", Name: "TV"}}
	}
	t.Cleanup(func() { sendToPiikTV = previous })
	server := startFixture(t, "")
	endpoint := strings.TrimSuffix(server.URL(), "/client") + "/api/client-launcher/piik-tv"

	post := func(body string) (int, []piiktv.Target) {
		t.Helper()
		response, err := localPost(endpoint, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result struct {
			Sent []piiktv.Target `json:"sent"`
		}
		_ = json.NewDecoder(response.Body).Decode(&result)
		return response.StatusCode, result.Sent
	}

	other := "https://someone-else.trycloudflare.com/r/1234#v=AbCdEfGhIjKlMnOpQrStUA"
	if status, tvs := post(`{"link":"` + other + `"}`); status != http.StatusOK || len(tvs) != 1 || tvs[0].Name != "TV" {
		t.Fatalf("send = %d, %+v", status, tvs)
	}
	if status, tvs := post(`{"link":"http://192.168.0.20:8787/r/2222"}`); status != http.StatusNotFound || tvs == nil || len(tvs) != 0 {
		t.Fatalf("no TV = %d, %+v", status, tvs)
	}
	for _, body := range []string{
		`{"link":"javascript:alert(1)"}`,
		`{"link":"https://example.com/"}`,
		`{"link":"https://example.com/r/0123"}`,
		`{"link":"https://exa mple.com/r/1234"}`,
		`{"link":"https://example.com/r/1234","extra":1}`,
		`not json`,
	} {
		if status, _ := post(body); status != http.StatusBadRequest {
			t.Fatalf("%s accepted: %d", body, status)
		}
	}
	if strings.Join(sent, " ") != other+" http://192.168.0.20:8787/r/2222" {
		t.Fatalf("sent = %q", sent)
	}

	// Another page cannot make the App send links.
	request, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"link":"`+other+`"}`))
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound || len(sent) != 2 {
		t.Fatalf("cross-origin send = %d, %d sends", response.StatusCode, len(sent))
	}
}

// The launcher page learns the language of its Piik TV texts with the App state.
func TestLauncherStateHasPiikTVLanguage(t *testing.T) {
	server := startFixture(t, "")
	response, err := http.Get(strings.TrimSuffix(server.URL(), "/client") + "/api/client-launcher")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var state struct {
		PiikTvLanguage string `json:"piikTvLanguage"`
	}
	if json.NewDecoder(response.Body).Decode(&state) != nil || state.PiikTvLanguage != piiktv.Language() ||
		(state.PiikTvLanguage != "it" && state.PiikTvLanguage != "en") {
		t.Fatalf("piikTvLanguage = %q", state.PiikTvLanguage)
	}
}

func TestPiikTVValidLink(t *testing.T) {
	for link, want := range map[string]bool{
		"https://x.trycloudflare.com/r/1234#v=AbCdEfGhIjKlMnOpQrStUA": true,
		"http://192.168.0.8:8787/r/9999":                              true,
		"https://192.168.0.8:8787/r/1000?x=1":                         true,
		"https://x.example/r/123":                                     false,
		"ftp://x.example/r/1234":                                      false,
		"https://x.example/r/1234 extra":                              false,
		"https://x.example/r/1234" + strings.Repeat("a", 1100):        false,
	} {
		if piiktv.ValidLink(link) != want {
			t.Errorf("ValidLink(%q) = %v", link, !want)
		}
	}
}
