# VibePiik native UI — host logic map (from the web client, Piik v2.5.3)

## Processes in piik-app
- loopback control server 127.0.0.1:39721-39730 (`internal/app/loopback`) → `nativecontrol.Session` per connection.
- launcher server 127.0.0.1:<random>/client (`internal/app/launcher`).
- room server `serverapp` on :8787 after Launch (local: 127.0.0.1 http + https LAN self-signed; link: 0.0.0.0 + cloudflared). No SFU.

## Launcher API (same-origin JSON POST, Host must be loopback:port)
- GET /api/client-launcher → {sites, startupSite, localAccessPassword, version, revision, controlPort, target, starting, detail, reason, lan{addresses,selected}, piikTvLanguage, modes, defaultMode, debug}
- POST /api/client-launcher/launch {mode local|link, language zh|en|vis, localAccessPassword, debug?, lanAddress?} → 200 {target} (blocks) / 503 {error,detail,reason} / 409
- POST /site {site, allowed}; POST /startup-site {site}; POST /debug {}; POST /piik-tv {link}
- target = http://127.0.0.1:8787/?piik-client-port=<port>#piik-client=1&client-access=<password>
- Poll every 750 ms while starting.

## Host page (HostPage.tsx) — to port
- Auth: GET/POST /api/site-access {password} (cookie), GET /api/capabilities.
- Room: POST /api/rooms {codeEntryPolicy open|private, roomPassword?, preferredRoomId?} → {roomId, hostToken, inviteUrl, codeEntryPolicy};
  replacement POST /api/rooms/{id}/replacement (Bearer hostToken); access POST /api/rooms/{id}/access
  {action: set-code-entry-policy | rotate-viewer-grant | revoke-viewer-grant | set-viewer-password}.
- Invite link <publicBase>/r/<code>#v=<grant>; TV: POST /api/piik-tv/send {link}.
- Signaling WS /signal (Origin http://127.0.0.1:8787 + site cookie): authenticate {protocol piik-v24, roomId, role host, token,
  clientId, roomSession, roomOnly|shareGeneration, sharingPaused, qualitySettings, routePolicy, viewerPresence, displayName};
  keepalive signaling-challenge 5 s; reconnect backoff ≤8 s. Structs in internal/server/protocol (client.go, server.go).
  Send: start-sharing, stop-sharing, set-sharing-paused, set-quality-settings, signal, route-failed, route-transport-connected,
  reset-sender-quality, sender-quality-evidence, set-display-name, subscribe-remote-input, set-co-play-presence, abandon-room…
  Receive: authenticated, sharing-started, sharing-start-failed, route-update, route-status, signal, restart-request,
  viewer-presence, viewer-quality-evidence, pause-sharing-source, remote-input-peers, room-closed, error.
- Native control (in-process: nativecontrol.Session.Handle(ctx, json) / Events()): hello, capture-options, list-sources,
  source-preview (base64 BMP), start-share {shareId, source, audio, excludeAudio, showCaptureBorder, adapterIndex, encoderIndex,
  edgeCapacity, profile, codec}, update-share, replace-share-source, pause-share, stop-share, prepare-edge → edge-offer,
  edge-answer, edge-candidate, close-edge, prepare-punch/run-punch/close-punch, set-microphone, set-share-volume,
  remote-control commands. Events: capture-state, audio-state, edge-candidate, edge-state, edge-path, edge-quality,
  remote-control-state, share-ended.
- Viewer media: route-update/authenticated assignment.childPeerIds → per child prepare-edge → offer via signal
  {kind:description, connectionId}; answer → edge-answer; candidates both ways (NAT batch in link mode).
- Quality presets 720p30, 1080p30, 1080p60 (default); persisted GET/PUT /quality-preference (X-Piik-Instance).
- Storage: preferred room code (piik:host-room-preference), creation profile, display name, room preferences, ui lang/mode/theme.

## Port size
Full parity ≈ 7–8k TS lines; minimum viable native host ≈ 3.5–4.5k.
