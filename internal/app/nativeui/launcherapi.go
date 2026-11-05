package nativeui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LauncherState is GET /api/client-launcher (internal/app/launcher/server.go).
type LauncherState struct {
	Sites               []string `json:"sites"`
	StartupSite         string   `json:"startupSite"`
	LocalAccessPassword string   `json:"localAccessPassword"`
	Version             string   `json:"version"`
	Revision            string   `json:"revision"`
	PackageTarget       string   `json:"packageTarget"`
	ControlPort         int      `json:"controlPort"`
	Target              string   `json:"target"`
	Starting            bool     `json:"starting"`
	Detail              string   `json:"detail"`
	Reason              string   `json:"reason"`
	LAN                 *struct {
		Addresses []struct {
			Address string `json:"address"`
			Name    string `json:"name"`
		} `json:"addresses"`
		Selected string `json:"selected"`
	} `json:"lan"`
	PiikTvLanguage string   `json:"piikTvLanguage"`
	Modes          []string `json:"modes"`
	DefaultMode    string   `json:"defaultMode"`
	Debug          *bool    `json:"debug"`
}

// LaunchError is a failed launch: 503 {error, detail, reason}.
type LaunchError struct {
	Message, Detail, Reason string
	Status                  int // HTTP status (409: another launch owns the App)
}

func (e *LaunchError) Error() string { return e.Message }

// launcherClient calls the App launcher's loopback API exactly like the web
// launcher page: same-origin JSON requests.
type launcherClient struct {
	base   string // http://127.0.0.1:<port>
	client *http.Client
}

func newLauncherClient(launcherURL string) *launcherClient {
	base := strings.TrimSuffix(strings.TrimSuffix(launcherURL, "/client"), "/")
	if index := strings.Index(base, "/client"); index > 0 {
		base = base[:index]
	}
	return &launcherClient{base: base, client: &http.Client{Timeout: 10 * time.Minute}}
}

func (c *launcherClient) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Origin", c.base)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if out != nil && len(data) > 0 {
		_ = json.Unmarshal(data, out)
	}
	return response.StatusCode, nil
}

func (c *launcherClient) State(ctx context.Context) (LauncherState, error) {
	var state LauncherState
	status, err := c.do(ctx, http.MethodGet, "/api/client-launcher", nil, &state)
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf("launcher state: HTTP %d", status)
	}
	return state, err
}

func (c *launcherClient) Launch(ctx context.Context, mode, language, password, lanAddress string, debug bool) (string, error) {
	body := map[string]any{"mode": mode, "language": language, "localAccessPassword": password}
	if debug {
		body["debug"] = true // diagnostics were switched on in the launcher
	}
	if mode == "local" && lanAddress != "" {
		body["lanAddress"] = lanAddress
	}
	var result struct {
		Target string `json:"target"`
		Error  string `json:"error"`
		Detail string `json:"detail"`
		Reason string `json:"reason"`
	}
	status, err := c.do(ctx, http.MethodPost, "/api/client-launcher/launch", body, &result)
	if err != nil {
		return "", &LaunchError{Message: "launcher unavailable", Detail: err.Error(), Reason: "launcher"}
	}
	if status == http.StatusOK && result.Target != "" {
		return result.Target, nil
	}
	return "", &LaunchError{Message: result.Error, Detail: result.Detail, Reason: result.Reason, Status: status}
}

// SendTV posts a room link to the PIIK TV apps; names are the TVs reached.
func (c *launcherClient) SendTV(ctx context.Context, link string) ([]string, int, error) {
	var result struct {
		Sent []struct {
			Address string `json:"address"`
			Name    string `json:"name"`
		} `json:"sent"`
	}
	status, err := c.do(ctx, http.MethodPost, "/api/client-launcher/piik-tv", map[string]string{"link": link}, &result)
	if err != nil {
		return nil, 0, err
	}
	var names []string
	for _, tv := range result.Sent {
		if tv.Name != "" {
			names = append(names, tv.Name)
		} else {
			names = append(names, tv.Address)
		}
	}
	if status == http.StatusOK && len(names) == 0 {
		return nil, status, errors.New("no TV answered")
	}
	return names, status, nil
}

func (c *launcherClient) EnableDebug(ctx context.Context) error {
	status, err := c.do(ctx, http.MethodPost, "/api/client-launcher/debug", map[string]any{}, nil)
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf("debug: HTTP %d", status)
	}
	return err
}

// SetSite approves (allowed) or revokes a Piik Site's access to this App.
func (c *launcherClient) SetSite(ctx context.Context, site string, allowed bool) error {
	_, err := c.do(ctx, http.MethodPost, "/api/client-launcher/site", map[string]any{"site": site, "allowed": allowed}, nil)
	return err
}

// SetStartupSite chooses the site opened when the App starts ("" for none).
func (c *launcherClient) SetStartupSite(ctx context.Context, site string) error {
	status, err := c.do(ctx, http.MethodPost, "/api/client-launcher/startup-site", map[string]any{"site": site}, nil)
	if err == nil && status != http.StatusOK {
		err = fmt.Errorf("startup site: HTTP %d", status)
	}
	return err
}
