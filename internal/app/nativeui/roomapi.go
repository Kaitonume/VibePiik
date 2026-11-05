package nativeui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/TNTcraftHIM/Piik/internal/server/protocol"
)

// roomTarget is the App's room entry URL:
// http://127.0.0.1:8787/?piik-client-port=<port>#piik-client=1&client-access=<password>
type roomTarget struct {
	Origin   string // http://127.0.0.1:8787
	Password string // local access password ("" when open)
}

func parseRoomTarget(target string) (roomTarget, error) {
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return roomTarget{}, fmt.Errorf("room entry %q is not an http URL", target)
	}
	fragment, _ := url.ParseQuery(parsed.Fragment)
	return roomTarget{Origin: parsed.Scheme + "://" + parsed.Host, Password: fragment.Get("client-access")}, nil
}

type httpStatusError struct {
	Path   string
	Status int
	Body   string
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d %s", e.Path, e.Status, strings.TrimSpace(e.Body))
}

// roomAPI is the room server's HTTP API, with the site admission cookie the
// page would hold.
type roomAPI struct {
	target roomTarget
	client *http.Client
}

func newRoomAPI(target roomTarget) *roomAPI {
	jar, _ := cookiejar.New(nil)
	return &roomAPI{target: target, client: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
}

func (a *roomAPI) do(ctx context.Context, method, path string, body any, bearer string, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, a.target.Origin+path, reader)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Origin", a.target.Origin)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return response.StatusCode, httpStatusError{Path: path, Status: response.StatusCode, Body: string(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return response.StatusCode, err
		}
	}
	return response.StatusCode, nil
}

// Admit obtains the site admission cookie with the local access password.
func (a *roomAPI) Admit(ctx context.Context) error {
	var state struct {
		Required bool `json:"required"`
		Admitted bool `json:"admitted"`
	}
	if _, err := a.do(ctx, http.MethodGet, "/api/site-access", nil, "", &state); err == nil && (state.Admitted || !state.Required) {
		return nil
	}
	if a.target.Password == "" {
		return nil
	}
	_, err := a.do(ctx, http.MethodPost, "/api/site-access", map[string]string{"password": a.target.Password}, "", nil)
	return err
}

func (a *roomAPI) CreateRoom(ctx context.Context, preferred string) (protocol.CreateRoomResponse, error) {
	body := map[string]any{"codeEntryPolicy": protocol.CodeEntryOpen}
	if preferred != "" {
		body["preferredRoomId"] = preferred
	}
	var room protocol.CreateRoomResponse
	_, err := a.do(ctx, http.MethodPost, "/api/rooms", body, "", &room)
	return room, err
}

// SendTV is POST /api/piik-tv/send {link}: the TVs that took the link.
func (a *roomAPI) SendTV(ctx context.Context, link string) ([]string, int, error) {
	var result struct {
		Sent []struct {
			Address string `json:"address"`
			Name    string `json:"name"`
		} `json:"sent"`
	}
	status, err := a.do(ctx, http.MethodPost, "/api/piik-tv/send", map[string]string{"link": link}, "", &result)
	if err != nil {
		return nil, status, err
	}
	var names []string
	for _, tv := range result.Sent {
		if tv.Name != "" {
			names = append(names, tv.Name)
		} else {
			names = append(names, tv.Address)
		}
	}
	return names, status, nil
}

// signalURL is the room server's signaling WebSocket.
func (a *roomAPI) signalURL() string {
	return "ws" + strings.TrimPrefix(a.target.Origin, "http") + "/signal"
}

// cookieHeader is the admission cookie for the WebSocket handshake.
func (a *roomAPI) cookieHeader() string {
	parsed, err := url.Parse(a.target.Origin)
	if err != nil {
		return ""
	}
	var parts []string
	for _, cookie := range a.client.Jar.Cookies(parsed) {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}
