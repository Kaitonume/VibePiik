package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/TNTcraftHIM/Piik/internal/piiktv"
)

// PiikTV: the Host page's "Manda URL all'app PIIK TV" button. The local App finds
// the Piik TV apps open on this computer's LANs (a small HTTP receiver on TCP
// 8790) and hands them the room link, so the page needs no TV address and makes
// no cross-origin request into the LAN.

type piikTVSendResult struct {
	Sent []piiktv.Target `json:"sent"`
}

func (s *Server) handlePiikTVSend(writer http.ResponseWriter, request *http.Request) {
	noStoreJSON(writer)
	if !allowMethod(writer, request, http.MethodPost) {
		return
	}
	// Only the Host's own page on this computer, never a visitor through the tunnel.
	if !piikTVLocalRequest(request) || !s.allowedRequestOrigin(request) {
		sendJSON(writer, http.StatusForbidden, errorBody{"Only the local Piik page can send to Piik TV"})
		return
	}
	body, err := readJSONBody(request)
	if err != nil {
		sendJSON(writer, http.StatusBadRequest, errorBody{"Invalid request"})
		return
	}
	var input struct {
		Link string `json:"link"`
	}
	if json.Unmarshal(body, &input) != nil || !piiktv.ValidLink(input.Link) {
		sendJSON(writer, http.StatusBadRequest, errorBody{"Invalid room link"})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), piiktv.SendTimeout)
	defer cancel()
	result := piikTVSendResult{Sent: piiktv.SendAll(ctx, input.Link)}
	status := http.StatusOK
	if len(result.Sent) == 0 {
		status = http.StatusNotFound
	}
	sendJSON(writer, status, result)
}

// handlePiikTVLanguage tells the Host page which language to use for the Piik TV
// texts ("it" or "en"): the Windows display language, like the App console.
func (s *Server) handlePiikTVLanguage(writer http.ResponseWriter, request *http.Request) {
	noStoreJSON(writer)
	if !allowMethod(writer, request, http.MethodGet) {
		return
	}
	if !piikTVLocalRequest(request) {
		sendJSON(writer, http.StatusForbidden, errorBody{"Only the local Piik page can read this"})
		return
	}
	sendJSON(writer, http.StatusOK, map[string]string{"language": piiktv.Language()})
}

// piikTVLocalRequest accepts loopback connections addressed to a loopback host.
// cloudflared also connects from loopback, but it forwards the public Host and
// its Cf- headers.
func piikTVLocalRequest(request *http.Request) bool {
	remote, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return false
	}
	if ip := net.ParseIP(remote); ip == nil || !ip.IsLoopback() {
		return false
	}
	for _, header := range []string{"Cf-Connecting-Ip", "Cf-Ray", "X-Forwarded-For", "Forwarded"} {
		if request.Header.Get(header) != "" {
			return false
		}
	}
	host := request.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
