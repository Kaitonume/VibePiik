package launcher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/TNTcraftHIM/Piik/internal/piiktv"
)

// sendToPiikTV finds the open Piik TV apps and hands them the link; tests replace it.
var sendToPiikTV = piiktv.SendAll

// handlePiikTV sends a link typed on the launcher page to the Piik TV apps on
// this computer's LANs. It can be any Piik room, also someone else's, and works
// before this App starts a room. requestAllowed has already limited POSTs to
// this loopback page's own origin with a JSON body.
func (server *Server) handlePiikTV(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBytes)
	var input struct {
		Link string `json:"link"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || !piiktv.ValidLink(input.Link) {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid room link"})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), piiktv.SendTimeout)
	defer cancel()
	stop := context.AfterFunc(server.ctx, cancel)
	defer stop()
	sent := sendToPiikTV(ctx, input.Link)
	status := http.StatusOK
	if len(sent) == 0 {
		status = http.StatusNotFound
	}
	writeJSON(response, status, map[string]any{"sent": sent})
}
