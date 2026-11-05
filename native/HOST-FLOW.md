# Piik host page in App mode — porting spec for native Go

Scope: what `src/client/pages/HostPage.tsx` and the modules it uses do when the page is opened by the
desktop App at `http://127.0.0.1:8787/?piik-client-port=<port>#piik-client=1&client-access=<password>`
and captures through the native control server (`internal/app/loopback` + `internal/app/nativecontrol`).
This spec does not cover: browser `getDisplayMedia` capture, SFU (`media/host-sfu-route.ts`, `sfu/*`),
chat, voice, comics, remote control and hints. Where one of these affects the core flow, it gets one line.

Conventions: `file:line` is relative to the repo root. "Page" means the TS host page. "App" means the
Go process: the loopback control server plus nativecontrol/nativehost/mediaedge. "Server" means the
room server (`internal/server`, port 8787). "Port" means the Go program you are writing. JSON shapes are
quoted compactly. `?` marks an optional field. `|` separates enum values.

---

## 0. Big picture (one paragraph)

The page does five things:
- It authenticates to the room server with a site cookie and creates or reuses a room over HTTP.
- It opens one signaling WebSocket (`/signal`) and keeps one native control WebSocket
  (`ws://127.0.0.1:<port>/control`).
- It asks the App to capture and encode (`start-share`).
- For every child viewer that the room server assigns to the host, it asks the App for a WebRTC offer
  (`prepare-edge`) and relays SDP and ICE between the App and signaling. The App owns the actual
  PeerConnections (pion, `internal/app/mediaedge`).
- It also opens a browser-only preview PeerConnection to the App (`prepare-local-edge`). The page needs it
  to start sharing, but viewers do not need it (§4.3).

---

## 1. Page boot in App mode

### 1.1 URL bootstrap (`src/client/lib/session.ts:166-229`, called from `src/client/App.tsx:49-60`)
- App-side construction:
  - `launcher.LocalEntryURL("http://127.0.0.1:<8787>/", config.LocalAccessPassword, control.Endpoint().Port)`
    (`internal/app/app.go:643-647`).
  - `EntryURL` sets `#piik-client=1` and `?piik-client-port=<39721..39730>`.
  - `LocalEntryURL` adds `#client-access=<password>` only when the password is non-empty
    (`internal/app/launcher/entry.go:11-46`).
  - The fragment may also carry `piik-lang`, `piik-mode` (`vis|text`), `piik-theme` (`light|dark|system`) and
    `?piik-preferences=copy,theme` (`session.ts:149-164`). These are cosmetic.
- `takeClientLaunchBootstrap()`:
  - Parses the fragment as URLSearchParams. `piik-client === "1"` sets `clientActivatedInPage = true`.
  - `piik-client-port` must be an integer in [39721, 39730] (`session.ts:24-27`,
    `src/client/native/discovery-ports.ts:1-2`). It goes to sessionStorage `piik:client-runtime-port`.
    Out-of-range values clear that key.
  - Writes localStorage `piik:client-launch:v1 = "1"`. Later loads without the fragment still count as App
    launches (`isClientActivated`, `session.ts:29-32`).
  - Returns `accessToken = client-access || null`.
  - Strips every bootstrap key from the URL with `history.replaceState`.
- `launchedByClient` (React prop) = `isClientActivated()` (`App.tsx:105`, `:278`, `:401`).

### 1.2 Site access + capabilities (`App.tsx:270-410`, `src/client/lib/api.ts`)
Both run in parallel (`loadAccess`, `App.tsx:285-290`):
- If `accessToken` is set: `POST /api/site-access`.
  - Headers: `Accept: application/json`, `Content-Type: application/json`. Body: `{"password":"<client-access>"}`.
  - See `api.ts:102-125`.
- Otherwise: `GET /api/site-access` with `cache:"no-store"` (`api.ts:63-81`).
- `GET /api/capabilities` with `Accept: application/json` (`api.ts:83-100`).

Server side (`internal/server/app/routes.go:165-217`, `internal/server/app/access.go`):
- Response body: `{required:boolean, authenticated:boolean}`.
  - `required` means a password is configured. With no password the response is `{false,true}` and no cookie
    is set (`routes.go:206-208`).
- POST needs an allowed `Origin` header (`routes.go:188-191`). Allowed origins are
  `http://localhost:8787` and `http://127.0.0.1:8787`, plus LAN origins and the public base
  (`internal/server/config/local.go:102-109`). Otherwise the response is 403 `{"error":"Origin not allowed"}`.
- The body must be exactly `{"password": "<string>"}`. A `Bearer` header is accepted when there is no body.
- Wrong password: 401 plus `WWW-Authenticate: Bearer`. Right password: 200 plus `Set-Cookie`.
- Cookie (`access.go:137-162`):
  - Format: `piik-site-access=v1.<expiresUnixSec>.<HMAC-SHA256 b64url 43 chars>; Path=/; Max-Age=86400; HttpOnly; SameSite=Strict`.
  - The `__Host-piik-site-access` + `Secure` variant applies only to the configured https host.
  - Requests to `http://127.0.0.1:8787` get the plain name (`access.go:55-69`).
  - The signing key is random per process, so cookies die when the App restarts.
  - An authenticated `GET /api/site-access` re-issues the cookie (sliding TTL, `routes.go:174-177`).
- Gate states: `ready` means `!required || authenticated`. A 401 means "required" (password form).
  Anything else means "unavailable" (retry button) (`App.tsx:93-97, 292-319`).
- Renewal: while ready and `required`, the page sends `GET /api/site-access` every
  `SITE_ACCESS_RENEWAL_INTERVAL_MS = 3_600_000` and drops to the gate if access is lost (`App.tsx:85, 321-349`).

Capabilities (`routes.go:74-96`; schema `src/shared/protocol.ts:232-247`; Go `internal/server/protocol/types.go:187`).
The App runtime always returns:
`{sfu:false, sfuOnly:false, natPrediction:<link-mode? true:false>, roomInteractions:true, roomVoice:true,
roomVoiceListeningPresence:true, roomVoiceBitrate:true, relayVolunteer:true, roomReactionIds:[...], remoteInput:true,
viewerRouteSelection:true, audioPresence:true, coPlayPresence:true, hostRoomSession:true}`.
- The App's Local composition has no SFU.
- `natPrediction` is true only in link mode: Local config enables it only with 2 auxiliary STUN URLs
  (`local.go:63-74, 122`; `internal/app/config/public_stun.go:12-27`).

HostPage props are derived from these values (`App.tsx:396-409`):
`hostRoomSessionAvailable = capabilities.hostRoomSession` (always true here), `sfuAvailable=false`,
`sfuOnly=false`, `natPredictionAvailable=capabilities.natPrediction`, `roomInteractionsAvailable=true`.

`ApiError` mapping (`api.ts:28-44`): 403 "Origin not allowed", 503 "Room capacity reached", 401 → gate.

### 1.3 Other boot-time HTTP calls to the App (loopback, port 39721..39730)
- **Discovery** (`src/client/native/client.ts:146-211`):
  - Request: `GET http://127.0.0.1:<port>/health` with `cache:"no-store"`.
  - Only the selected port is probed (`clientRuntimePort()`). Without one, the page probes all 10 ports in
    parallel (`Promise.any`) with an 8 s deadline (`REQUEST_TIMEOUT_MS = 8_000`, `client.ts:52`).
  - Validation: `{protocol, service:"piik-client", port}` must equal the probed port, and `protocol` must be 10.
    Any other protocol value raises `NativeCompatibilityError`.
- **Health body** (`internal/app/loopback/server.go:73-109, 410-435`; TS `src/client/native/wire.ts:28-66`):
  `{version, revision, packageTarget, protocol:10, service:"piik-client", port, instanceToken(43 b64url),
  nativeMedia:{video, hardwareH264, softwareVP8, processAudio, systemAudio, processAudioExclusion?, microphone?,
  shareVolume?, captureBorderControl?, sourceCancellation?, captureFailureDetails?, remoteInput?,
  remoteStopShortcut?, remoteCoPlay?, receiverReuse?, maxRelayChildren?, maxAudioBitrate?, maxVideoResolution?,
  maxVideoPixels?, maxVideoMacroblocksPerSecond?, maxVideoFramerate?, maxVideoBitrate?},
  qualityPreference?:true, activationURL?}`.
- **Presentation** (`App.tsx:107-115`, `client.ts:213-227`):
  - Runs when `launchedByClient`, on each lang/vis change: `/health`, then
    `POST http://127.0.0.1:<port>/presentation`.
  - Body `{"language":"zh"|"en"|"vis"}`. The App replies 204 (`loopback/server.go:380-408`). Cosmetic (console language).
- **Quality preference** (`src/client/native/quality-preference.ts:16-75`, used at `HostPage.tsx:688-698, 1719-1743`):
  - Runs only when `launchedByClient` and `health.qualityPreference`.
  - `GET http://127.0.0.1:<port>/quality-preference` with headers `Content-Type: application/json` and
    `X-Piik-Instance: <health.instanceToken>`. Response: a QualitySettings JSON or `null`. Timeout 4 s.
  - On load (`loadAppQualityPreference`, `HostPage.tsx:1728-1743`): the saved value replaces the default only
    if the user has not edited quality, no share is active, no room mutation is running, and
    `supportsCaptureProfile(saved)` holds.
  - Saves: `PUT` with the same headers and a QualitySettings body, answered with 204. Writes are coalesced so
    only the latest value is sent (`save/flush`).
  - Without the App store, the fallback is localStorage `piik:quality-preference:v1`
    (`src/client/lib/quality-preference.ts:10-31`).
  - The server handler checks `X-Piik-Instance` (403 when wrong) (`internal/app/loopback/quality_preference.go:13-53`).
- The header's `AppConnectionButton` re-reads `/health` on window focus (cosmetic,
  `components/living/AppConnectionButton.tsx:45-72`).

Loopback HTTP guard (`loopback/server.go:625-640, 667-677`):
- `Host` must be `127.0.0.1:<port>` or `localhost:<port>`.
- `Origin` may be empty, an approved site, or `http://127.0.0.1:<port>`/`http://localhost:<port>`.
- The page origin `http://127.0.0.1:8787` is approved by `allowedOrigins(sites, 8787)` (`internal/app/app.go:695-701`).

### 1.4 Native control WebSocket (`client.ts:247-292`, `loopback/server.go:443-604`)
- **When.** The page connects lazily: on opening the source picker, or when sharing starts
  (`NativeClientConnection.acquire`, `src/client/native/client-connection.ts:15-39`; `HostPage.tsx:642, 1462-1480`).
  - The connection is released when nothing owns it (`releaseUnusedNativeClient`, `HostPage.tsx:1474-1480`):
    no open picker, no native share, no source switch.
  - Closing the control connection stops the App's share (`extension.Close()`, `loopback/server.go:523-524`).
- **URL.** `ws://127.0.0.1:<health.port>/control`. The only subprotocol offered is `piik-client-v10.<instanceToken>`.
  - After open, the page checks `socket.protocol` equals that string. Open timeout 8 s.
  - The server rejects the upgrade with 409 when 2 control sessions already exist (`maxControlSessions = 2`).
  - The Origin must be allowed. coder/websocket `OriginPatterns` includes `http://127.0.0.1:<port>`.
    An empty Origin is allowed for native tooling (`loopback/server.go:653-665`).
- **Handshake.** The first frame must be text, within `helloTimeout = 5 s`.
  - Request: `{"version":10,"id":"<opaque>","type":"hello"}`. It is decoded with DisallowUnknownFields.
  - Reply: `{"version":10,"id":"<same>","type":"ready"}` (`loopback/protocol.go:42-58`).
- **Envelope.**
  - Request: `{"version":10,"id":<id>,"type":<cmd>, ...fields}`, where `id = createOpaqueId()`
    (32 lowercase hex chars, `src/client/lib/opaque-id.ts:1-6`; regex `^[A-Za-z0-9_-]{8,256}$`).
  - Response: same `id`, with a reply `type` or `{"type":"request-failed","code":"operation-failed"|"source-canceled"}`.
  - Events have no `id` (`client.ts:837-886`). Messages are at most 256 KiB (`MaxControlMessageBytes`).
- **Liveness.** `ping` → `pong` exists (`client.ts:308`, `loopback/server.go:575-587`). HostPage never calls it.
- **Timeouts (client side).** The default per-request timeout is 8 s. These requests have no timeout (`null`):
  `start-share` when `source.kind==="picker"`, `update-share`, `set-microphone`, `set-remote-control-policy`,
  and `replace-share-source` with a picker.
  - On timeout the request is rejected as `NativeRequestError("timeout")`. The page usually then calls
    `nativeConnection.discard(client)`, which closes the socket.
- **Strictness.**
  - Page side: an event that fails `nativeEventSchema` closes the socket with 1008 (`client.ts:869-881`).
    Exception: an invalid `edge-quality`/`publication-quality` sample is just dropped.
  - App side: nativecontrol uses `decodeStrict` everywhere. An unknown field or bad identity is a
    `protocolViolation`, and the App closes control with StatusPolicyViolation (`nativecontrol/session.go:114-133`,
    `loopback/server.go:592-596`). Optional flags must be omitted, not sent as `false`
    (`client.ts:348-362` comments "Older Apps reject unknown command fields").
- **Async completion.** `start-share` and `replace-share-source` complete later. `Handle` returns nil, and the
  response comes back through `Events()` as `loopback.ControlEvent` (`nativecontrol/session.go:631-672`).
  The client matches responses by `id` only.
- **In-process option for the port.** `nativecontrol.New(...)` and
  `Session.Handle(ctx, json) (any, error)` / `Events() <-chan any` / `Close()` are exactly what the loopback
  server drives (`internal/app/app.go:691`, `loopback/server.go:47-54`). A native host can call them directly
  and skip the socket, discovery and subprotocol. If you do this:
  - Honour `ControlEvent.Current()` (drop when false) and `BeforeWrite()` (call before delivering).
  - Treat `error != nil` as "connection dead".
  - Answer `hello`/`ping` yourself, or skip them.

---

## 2. Room creation and reuse

### 2.1 Stored room restore (mount)
- On mount, `readHostRoom()` runs (`HostPage.tsx:460-472`, `session.ts:248-263`).
- It reads sessionStorage `piik:host-room:v1`, then the in-memory claim, then localStorage `piik:host-room:v1`.
  localStorage is read only if `navigator.locks` exists.
- Value: `{roomId, hostToken, inviteUrl}`, where `inviteUrl` is canonical (no hash, no query).
- The room is claimed with a Web Lock `piik:host-room:<roomId>:<sha256hex(hostToken)>` using `ifAvailable`
  (`session.ts:327-385`). This prevents two tabs from hosting the same room. If the lock is busy, the stored
  room is dropped from this tab.
- `hostRoomFromStored` (`HostPage.tsx:299-313`): `codeEntryPolicy = null` until authenticated.
  `inviteUrl = canonicalUrl + "#v=" + sessionStorage piik:viewer-grant:<roomId>` when present, else `null`.
- If a room was restored and `hostRoomSessionAvailable`, signaling connects immediately in room-only mode
  (`HostPage.tsx:476-482`, §3).
- **No room is auto-created at boot.** The placeholder shows a Create button, `RoomCodePlaceholder onCreate`
  calling `createIdleRoom` (`HostPage.tsx:4284-4288`). Otherwise a room is created when sharing starts (§4.4).

### 2.2 `POST /api/rooms` (`api.ts:127-163`; server `routes.go:302-346`)
- Request headers: `Accept: application/json`, `Content-Type: application/json`, plus browser-supplied `Origin`
  and `Cookie`.
- Server checks:
  - `Origin` must be allowed; otherwise 403.
  - When a password is configured, the site cookie is required; otherwise 401. A Bearer site password is NOT
    accepted here (`routes.go:315-320`).
- Body (strict, `protocol.ts:1328-1334`; Go `internal/server/protocol/http.go:9-46`):
  `{codeEntryPolicy:"open"|"private", roomPassword?: string(1..64 printable ASCII \x21-\x7e), preferredRoomId?: "NNNN"}`.
  - The page omits `roomPassword` when it is null and omits `preferredRoomId` when absent.
- Response: 201 `{roomId:"[1-9]\d{3}", hostToken:"[A-Za-z0-9_-]{32,128}", inviteUrl, codeEntryPolicy}`
  (`protocol.ts:1318-1326`; `routes.go:392-399`).
- Errors (`routes.go:465-476`): 503 "Room capacity reached" / "Room creation unavailable", 400 invalid.
- Defaults come from `readCreationProfile()`, localStorage `piik:host-creation-profile:v1`
  `{codeEntryPolicy, roomPassword|null}`. Default `{codeEntryPolicy:"open", roomPassword:null}`
  (`src/client/lib/creation-profile.ts:7-43`).
- `preferredRoomId` = localStorage `piik:host-room-preference:v1` `{roomId}` (`session.ts:387-430`).
  - Written by `writePreferredRoom` on every host `authenticated`, on endSharing, and on replacement
    (`HostPage.tsx:1048, 1136, 2758`).
  - The server reuses the code only if it is free (`internal/server/room/store.go:917-925`).
- After creation (`createIdleRoom`, `HostPage.tsx:1070-1098`):
  - `hostRoomFromCreated` (`HostPage.tsx:315-327`) stores the grant (§2.5) and computes `canonicalUrl`.
  - `writeHostRoom` claims the lock, then writes both sessionStorage and localStorage `piik:host-room:v1`
    (`session.ts:432-475`).
  - It sets `viewerPasswordEnabled = roomPassword !== null`.
  - If ownership was lost meanwhile, `closeAbandonedRoom` runs (§2.4).

### 2.3 Replacement and access APIs (all `Authorization: Bearer <hostToken>`, Origin + cookie checked first, `routes.go:350-369`)
- **Replacement.** `POST /api/rooms/{id}/replacement` with body
  `{codeEntryPolicy, roomPassword?}` → 201 with a CreateRoomResponse for a new code and token
  (`api.ts:165-199`; `routes.go:219-261`).
  - 404 means invalid or unknown. The page then falls back to `createRoom(...)` when not sharing
    (`HostPage.tsx:1100-1202`).
  - When sharing, the page first calls `endSharing(..., notifyServer=false, keepRoomSession=false)`.
  - It then clears the old grant, writes the room, writes the preferred room and sets phase `ended|idle`.
- **Access.** `POST /api/rooms/{id}/access` (`api.ts:201-234`; `routes.go:263-300`). Body, one of
  (`protocol.ts:1344-1359`):
  - `{action:"set-code-entry-policy", policy}` → `{type:"code-entry-policy-updated", codeEntryPolicy, viewerPasswordEnabled}`
  - `{action:"rotate-viewer-grant"}` / `{action:"revoke-viewer-grant"}` → `{type:"viewer-grant-updated", viewerAuthorizationGeneration, inviteUrl|null}`
  - `{action:"set-viewer-password", password|null}` → `{type:"viewer-password-updated", enabled}`
  - Status codes: 404 means the room is gone, and the page calls forgetRoom plus endRoomSession. 409 means
    rejected. 401 means the gate (`HostPage.tsx:3504-3523`).
  - Handlers: `HostPage.tsx:3525-3681`. They update the creation profile on success.

### 2.4 `closeAbandonedRoom` (`HostPage.tsx:274-297`)
This disposes a room that was created but lost its owner.
- It opens a throwaway `SignalingClient` with host identity
  `{roomId, role:"host", token, clientId, shareGeneration:<fresh>, routePolicy:DEFAULT}`.
  - This is the legacy shape: no `roomSession`, so the server starts a share at authentication.
- It then calls `sendThenStop({type:"abandon-room"})`. If not sent within 15 s, it stops anyway
  (`signaling.ts:371-387`).

### 2.5 Identifiers and invite link
- **roomSession**: a boolean flag `roomSession:true` on host authenticate. It opts into the persistent room
  lifecycle: `start-sharing`/`stop-sharing` happen without leaving the room (`signaling.ts:670-721`;
  server `signal/server.go:1399-1428, 1444-1512`). It is set whenever `hostRoomSessionAvailable`
  (`HostPage.tsx:2651`).
- **clientId**: sessionStorage `piik:client-id:host:<roomId>`. A per-tab stable `createOpaqueId()`
  (32 hex), pattern `^[A-Za-z0-9_-]{8,128}$` (`session.ts:630-640`). The server uses it to recognise a
  reconnecting host and replace the old session.
- **shareGeneration**: a fresh `createOpaqueId()` per sharing start (`HostPage.tsx:2866`). Native
  `start-share.shareId` equals the shareGeneration for the initial share (`HostPage.tsx:1299`). When switching
  browser→native mid-share, the App shareId is a new opaque id while shareGeneration is unchanged.
- **Invite link**: built server-side, `signal.InviteURL(PublicBaseURL, roomId, viewerGrant)` =
  `<PublicBaseURL origin>/r/<roomId>#v=<grant>` (`internal/server/signal/server.go:312-325`).
  - Local mode: `PublicBaseURL = https://<selected LAN IPv4>:8787/` (self-signed TLS listener on that IP,
    `internal/app/app.go:558-569`; `local.go:84-100`).
  - Link mode: `PublicBaseURL = https://<cloudflared tunnel host>/` (`local.go:91-97`).
  - The LAN address is chosen with `lan.Select(addresses, options.LANAddress)` (`app.go:547-557`).
  - The grant is 22 chars `^[A-Za-z0-9_-]{21}[AQgw]$` (`protocol.ts:151-154`). It lives in sessionStorage
    `piik:viewer-grant:<roomId>` (`replaceViewerInvite`, `session.ts:547-575, 622-624`).
  - `canonicalUrl` = inviteUrl without hash or query.
  - UI link (`HostPage.tsx:563-565`): `includeInviteCredential (default true) ? inviteUrl : canonicalUrl`.
    It is blocked when canonical, private and passwordless. Copy goes through `navigator.clipboard`.
  - Piik TV button: `POST /api/piik-tv/send {link: inviteUrl ?? canonicalUrl}`, loopback-only. Response
    200 or 404 `{sent:[...]}` (`HostPage.tsx:3851`; `internal/server/app/piiktv.go:22-52`).

### 2.6 Storage keys (complete list touched by the core flow)
| key | store | value |
|---|---|---|
| `piik:client-launch:v1` | local | `"1"` |
| `piik:client-runtime-port` | session | `"39721".."39730"` |
| `piik:host-room:v1` | session+local | `{roomId,hostToken,inviteUrl(canonical)}` |
| `piik:host-room-preference:v1` | local | `{roomId}` |
| `piik:viewer-grant:<roomId>` | session | grant |
| `piik:client-id:host:<roomId>` | session | 32-hex |
| `piik:host-creation-profile:v1` | local | `{codeEntryPolicy, roomPassword|null}` |
| `piik:quality-preference:v1` | local | QualitySettings (fallback when no App store) |
| `piik:display-name:v1` | local | display name (`src/client/lib/display-name.ts:7`) |
| `piik:room-preferences:v1` | local | UI prefs (`src/client/lib/room-preferences.ts:5`) |

---

## 3. Signaling WebSocket

### 3.1 Transport
- **URL.** `new URL("/signal", location.href)` with ws/wss chosen to match http/https. Here that is
  `ws://127.0.0.1:8787/signal` (`src/client/lib/signaling.ts:77-81`). The query must be empty
  (`signal/server.go:341`).
- **Headers.** The browser sends `Origin: http://127.0.0.1:8787` and the `Cookie` with `piik-site-access`.
  - The server rejects a missing or disallowed Origin with HTTP 403 (`signal/server.go:351-355, 398-410`).
  - The upgrade records `siteAccessAuthenticated` from the cookie (`signal/server.go:365`;
    `internal/server/app/app.go:163-165`). A host without a valid cookie (when a password is set) gets
    `error AUTH_REQUIRED` and close 4003 (`signal/server.go:793-798`).
  - **Port note:** set both headers manually.
- **Server limits.** Read limit `MaxSignalBytes = 64 KiB`. The server pings every 30 s; reply with pong, which
  coder/websocket does while reading (`signal/server.go:57, 372-378`).
- **Authentication timing.** The server's authentication timeout is 5 s (`signal/server.go:53`).
  The client's is `AUTHENTICATION_TIMEOUT_MS = 8_000`, after which it closes with 4000.
- **Close codes** (`protocol.ts:12-18`): 1012 serviceRestart, 4001 sessionReplaced, 4002 clientReconnect,
  4003 authenticationFailed, 4004 viewerAccessRevoked. 1008 means invalid message.
- **Strictness.**
  - The server decodes strictly (`protocol.DecodeClientMessage`). Any invalid message gets
    `error INVALID_MESSAGE` and close 1008 (`signal/session.go:307-310`).
  - The page decodes every frame with zod `serverMessageSchema` (strict). A decode failure stops signaling with
    `STALE_CLIENT`, and the page does not reconnect (`signaling.ts:437-444, 616-619`).
  - Go has `protocol.DecodeServerMessage` for the port to reuse (`internal/server/protocol/server.go:246`).

### 3.2 `authenticate` (sent once per socket, on `open`; `signaling.ts:407-427`)
Message = `{type:"authenticate", protocol:"piik-v24", ...identity}`.
Schemas: `protocol.ts:885-912`; Go `internal/server/protocol/client.go:15-30`.

| field | value the page sends | notes |
|---|---|---|
| `type` | `"authenticate"` | |
| `protocol` | `"piik-v24"` (`SIGNALING_PROTOCOL`, `protocol.ts:11`) | |
| `roomId` | room code | |
| `role` | `"host"` | |
| `token` | hostToken | |
| `clientId` | §2.5 | |
| `roomSession` | `true` | always in App (`HostPage.tsx:2651`) |
| `roomOnly` | `true` on a fresh connection; removed once `startSharing()` runs | `roomOnly` requires `roomSession` |
| `shareGeneration` | absent in room-only mode; present after `startSharing` (reconnects carry it) | legacy mode (no room session) sends it from the start |
| `sharingPaused` | `false` initially; tracks pause | |
| `qualitySettings` | current QualitySettings (default 1080p60, §4.6) | |
| `routePolicy` | `{peerOnly: !sfuAvailable → true, topologyOptimization: !sfuOnly → true, natPrediction: capabilities.natPrediction}` | `HostPage.tsx:381-388`; server clamps (`signal/server.go:1221-1226`) |
| `viewerPresence` | `true` | needed to receive `viewer-presence` and to set the display name |
| `displayName` | localStorage name or default: `"Host"`/`"房主"` by locale, `"📺"` in vis mode | `display-name.ts:40-44`, `HostPage.tsx:2639-2643`; canonical NFC, at most 24 code points |

Fresh host example:
`{"type":"authenticate","protocol":"piik-v24","roomId":"4821","role":"host","token":"…","clientId":"<32hex>","roomSession":true,"roomOnly":true,"sharingPaused":false,"qualitySettings":{"resolution":{"width":1920,"height":1080},"maxFramerate":60,"maxBitrate":12000000,"degradationPreference":"balanced","maxAudioBitrate":128000},"routePolicy":{"peerOnly":true,"topologyOptimization":true,"natPrediction":false},"viewerPresence":true,"displayName":"Host"}`

Server reaction (`signal/server.go:784-1067`, `settleHostShare` `:1074-1149`):
- `roomOnly`: any lingering share is stopped, and the response carries `shareGeneration: null` and an empty
  assignment.
- With `roomSession` + `shareGeneration` (a reconnect while sharing): the share resumes if that generation is
  still current.
  - Exception: a generation that was already stopped (`routePolicy == nil`) is fenced, and the server returns
    `null` (`:1094-1097`).
- `authenticated` (host; `protocol.ts:1075-1126`; Go `protocol/server.go:15`):
  `{type:"authenticated", protocol, role:"host", peerId, maxViewers(≤20), endpointMediaCopyCapacity(1..3; Local = 2),
  hostOnline, hostPaused?, connectionId:null, iceConfig:{iceServers:[{urls:string|string[]}], natPredictionStunUrls:string[]},
  routePolicy, codeEntryPolicy, viewerAuthorizationGeneration, viewerPasswordEnabled, mediaMode:"peer-assisted",
  shareGeneration:string|null, routeRevision, routeAssignment:{upstream:{kind:"none"}, childPeerIds:[], sfuPublicationGeneration:null}, qualitySettings}`.
  - iceConfig in Local mode: `{iceServers:[{urls:["stun:<LAN>:8787"]}], natPredictionStunUrls:[]}`.
  - iceConfig in link mode: `{iceServers:[{urls:["stun:stun.cloudflare.com:3478","stun:stun.miwifi.com:3478","stun:stun.chat.bilibili.com:3478"]}], natPredictionStunUrls:["stun:stun.miwifi.com:3478","stun:stun.chat.bilibili.com:3478"]}`
    (`internal/server/config/ice.go:69-89`, `public_stun.go:18-24`).
- If the room's share is paused for this generation, `pause-sharing-source` follows immediately
  (`signal/server.go:1048-1054`). Then `viewer-presence` is sent.

### 3.3 Keepalive, watchdog, reconnect (`signaling.ts:577-614, 768-916`)
- **Watchdog.** It runs only while authenticated, the socket is OPEN and `document.visibilityState === "visible"`.
  - Every 5 s (`SIGNALING_CHALLENGE_INTERVAL_MS`) the page sends `{type:"signaling-challenge", sequence:n}` with
    n = 1, 2, …; the server echoes `{type:"signaling-challenge-response", sequence}` (`signal/server.go:1260-1280`).
    The server rate-limits to one challenge per 1 s.
  - If no reply arrives within 2 s, the page sends a second "confirm" challenge.
  - If that also gets no reply within 2 s, the page closes with 4002 and reconnects immediately
    (`replaceUnresponsiveSocket`).
  - A timer that fires more than 1 s late (sleep or throttling) re-arms instead of failing. A visibility change
    resets the watchdog.
- **Reconnect.** On close the page reconnects unless the code is one of 4001, 4003, 1008, 4004
  (`signaling.ts:68-75, 543-570`).
  - Delay = `min(8000, 500·2^min(attempt,4)) + rand·250` ms, i.e. 0.5, 1, 2, 4, 8, 8 … s. The attempt counter
    resets on `authenticated`.
  - A non-reconnect close reports `SESSION_REPLACED` (4001), `STALE_CLIENT` (1008) or `SIGNAL_TERMINATED`.
    The page then calls `endRoomSession`, and on `SESSION_REPLACED` also `forgetRoom(keepResumeHint=true)`
    (`HostPage.tsx:2667-2671`).
- **Errors.**
  - `error AUTH_REQUIRED`: the page retires the socket with 4002, then runs `GET /api/site-access` (8 s timeout).
    If the result is `required && !authenticated`, it stops and shows the gate (`onAccessRequired`); otherwise
    it schedules a reconnect (`signaling.ts:588-614`).
  - Fatal errors `INVALID_TOKEN`, `ROOM_NOT_FOUND`, `ROOM_ACCESS_DENIED`, `ROOM_FULL`, `HOST_ALREADY_CONNECTED`
    call `stop()` and are then delivered (`signaling.ts:43-49, 472-476`).
  - `error INVALID_MESSAGE` before authentication means `STALE_CLIENT`.
- **Status values** (`onStatus`): `connecting` on first start, `reconnecting` on later starts and after a close,
  `connected` on `authenticated`, `offline` on `stop()`.
- **`send()`.** It refuses (returns false) unless authenticated and the socket is OPEN (`signaling.ts:251-264`).
  The authenticate frame and challenges bypass it.

### 3.4 Host → server messages in the core flow (schemas `protocol.ts:935-1059`, Go `protocol/client.go`)
| message | when | page code |
|---|---|---|
| `{type:"start-sharing", shareGeneration, sharingPaused?, qualitySettings?, routePolicy}` | after native capture is up (§4.4); flushed on `authenticated` if it was queued | `signaling.ts:306-324, 645-668` |
| `{type:"stop-sharing", shareGeneration?}` | endSharing (roomSession keeps the socket) | `signaling.ts:327-341`, `HostPage.tsx:928-942` |
| `{type:"set-sharing-paused", shareGeneration, paused}` | pause toggle, after native `pause-share` | `signaling.ts:277-294` |
| `{type:"set-quality-settings", qualitySettings}` | quality change applied; re-sent on reauth if the server value differs | `signaling.ts:349-360, 723-744` |
| `{type:"signal", targetPeerId, payload}` | every SDP/ICE/punch payload to a viewer | `HostPage.tsx:2164-2167, 2301-2308` |
| `{type:"route-transport-connected", revision, connectionId}` | a prepared child's edge reaches `connected` (once) | `HostPage.tsx:2190-2192` |
| `{type:"route-failed", revision, phase:"prepare"\|"active", connectionId\|null}` | prepared or active child failed | `HostPage.tsx:2419-2449` |
| `{type:"sender-quality-evidence", ...}` | each native `edge-quality`/state snapshot (optional, §5.9) | `HostPage.tsx:2116-2130` |
| `{type:"reset-sender-quality"}` | page hidden/pagehide, source switched (optional) | `HostPage.tsx:723-747, 3081-3084` |
| `{type:"set-display-name", displayName}` | rename | `signaling.ts:266-275` |
| `{type:"set-audio-presence", shareGeneration, available}` | after `sharing-started` and when audio changes (capability `audioPresence`) | `signaling.ts:172-190` |
| `{type:"abandon-room"}` | only from `closeAbandonedRoom` | `HostPage.tsx:293` |

Side subscriptions sent on authenticate because the capabilities are on (out of scope, skippable):
- `subscribe-room-interactions` (`lib/room-interactions.ts:80`).
- `subscribe-room-voice` (`room-voice/session.ts:67`).
- `subscribe-remote-input` (`signaling.ts:500`).
- `set-co-play-presence` (`signaling.ts:152-158`).
- These only make the server send the matching extra messages.

### 3.5 Server → host messages in the core flow (`protocol.ts:1128-1315`) and what the page does
All are routed through `connectRoomSignal.onMessage` (`HostPage.tsx:2677-2798`). Media messages are forwarded
to `handleSignalMessage` (`:2451-2627`) only when `mediaActive`, meaning `activeGeneration` and
`shareGeneration` are set and `serverShareGenerationRef === shareGenerationRef` (`:2782-2789`).

- **`authenticated`** (`:2734-2759`):
  - Stores `iceConfigRef`, `hostPeerIdRef = peerId`, `endpointMediaCopyCapacityRef`, and
    `serverShareGenerationRef = shareGeneration`.
  - Saves the creation profile as `{codeEntryPolicy, roomPassword: viewerPasswordEnabled ? stored : null}` and
    sets `viewerPasswordEnabled`.
  - Merges into the room: `codeEntryPolicy`, and the invite restored from the stored grant
    (`session.ts:588-612`).
  - Calls `writeHostRoom` and `writePreferredRoom`, and pushes `updateIceConfig` to peers (a no-op for native).
  - If `mediaActive`: sets phase `live`, then `handleSignalMessage` (`:2460-2505`):
    - Discards the prepared child and adopts `routePolicy`.
    - `activeRouteRevision = routeRevision`.
    - On reauthentication, removes every peer that is not connected (lost offers/answers), commits quality
      (pending quality wins), and re-applies the profile to peers.
    - Calls `HostSfuRoute.resyncAuthoritative({revision, phase:"active", assignment: routeAssignment})`, which
      calls `reconcileChildren` (§5.4).
  - Signaling internals also run on `authenticated`: `reconcileHostShareState` (re-sends `start-sharing` if the
    server returned `shareGeneration:null` while the page wants one, or `stop-sharing` for a stale generation)
    and `reconcileHostQualityIntent` (`signaling.ts:670-744`).
- **`sharing-started`** `{shareGeneration, routePolicy, routeRevision, routeAssignment, qualitySettings, paused}`
  (Go `protocol/server.go:61`):
  - If it matches the current generation, sets `serverShareGenerationRef`, adopts `routePolicy`, commits
    quality and sets `activeRouteRevision`. It sets `sharingPaused = paused` and phase `live`, then resyncs the
    route with the assignment (`HostPage.tsx:2507-2526, 2760-2766`).
- **`sharing-start-failed`** `{shareGeneration, code:"FORBIDDEN"|"SERVER_ERROR"}`: if it matches,
  `endSharing(hostServerErrorKey, notifyServer=false)` (`:2767-2772`). Signaling reverts to room-only
  (`signaling.ts:517-523`).
- **`route-update`**: §5.2.
- **`signal`** `{fromPeerId, payload}`: §5.6.
- **`restart-request`** `{fromPeerId, connectionId, rebuild}`: §5.7.
- **`pause-sharing-source`** `{shareGeneration}`: the server believes the share is paused.
  - The page sends native `pause-share paused:true`, pauses all peers, `sharingPaused=true`,
    `confirmSharingPaused()`, and shows a notice (`:2527-2552`).
- **`viewer-presence`** `{viewers: Array<{role:"host",peerId,displayName,upstream:{kind:"none"}} | {role:"viewer",peerId,displayName,upstream:{kind:"none"|"peer"(peerId)|"sfu"},mediaReady?:true}>}`:
  replaces the presence list (`:2773-2777`; server `signal/presence.go:30-107`).
- **`viewer-quality-evidence`**: UI only (`:2587-2590`).
- **`room-closed`** `{reason:"host-ended"}`: `forgetRoom`, then `endRoomSession(..., "ended")` (`:2721-2725`).
- **`error`**:
  - `INVALID_TOKEN` before the first authentication, with replacement allowed:
    - If not sharing (or not wanting publication): forget the room and `endRoomSession("host.roomInvalid")`.
    - If sharing: create a replacement room (`createRoom` + new signaling + `startSharing`, `:2817-2849`).
  - Later `INVALID_TOKEN`: forget the room and end. `HOST_ALREADY_CONNECTED`: end the session.
  - Other codes become a notice (`:2691-2781`).
- **`sfu-config` / `sfu-signal`**: SFU only, ignored here.
- **`remote-input-peers`**: remote control only.
- Not sent to hosts: `host-status`, `quality-settings`, `route-policy`, `route-status`, `sharing-stopped`.
  These go to viewers (`signal/server.go:1109-1135`, `router.go:1611-1625`).

---

## 4. Sharing lifecycle with native capture

### 4.1 Source picker (`HostPage.tsx:1489-1566`)
1. If the client is not activated, the browser-only picker is shown (not our path).
2. `acquireNativeClient()` (discovery + control + hello). On `NativeCompatibilityError` the picker shows
   `incompatible`; on null it shows `unavailable`.
3. If `!nativeMedia.video || (!hardwareH264 && !softwareVP8)`, the picker shows `unsupported`.
4. The page sends `capture-options` and `list-sources` in parallel:
   - Request `{type:"capture-options"}` → `{type:"capture-options", adapters:[{index, name, identity, hardwareH264:[{index,name,identity}]}]}`
     (`wire.ts:113-129, 163-169`; Go `nativecapture/probe.go:31-42`).
   - Request `{type:"list-sources"}` → `{type:"source-list", sources: CaptureTarget[] (≤1024)}`.
     - CaptureTarget is one of:
       - `{kind:"window", sourceId:"<decimal>", pid, creationTime:"<decimal>", title}`
       - `{kind:"display", sourceId, title}`
       - `{kind:"picker", sourceId, title}`
     - See `wire.ts:80-111`; Go `nativecapture/stream.go:75-81`.
5. Capture path: `defaultNativeCapturePath(adapters, codec, softwareVP8)` (`native/capture-selection.ts:8-25`).
   - The codec is `videoCodecModeRef` = `"h264"` by default (`HostPage.tsx:390-391`).
   - It picks the first adapter that has `hardwareH264[0]`, giving `{adapterIndex, encoderIndex}`.
   - VP8 fallback is `{adapters[0].index, 0}`, only when codec is not `"h264"` and `softwareVP8` is available.
   - With the default `"h264"` mode and no hardware H.264 encoder, the result is `null` and the picker shows
     `unsupported`.
6. Previews (`HostPage.tsx:1629-1651`; `client.ts:330-342`):
   - Requests are serialized in one chain (`nativePreviewTailRef`) so thumbnails do not starve start-share.
   - Request `{type:"source-preview", source: CaptureTarget}` → `{type:"source-preview", sourceKey, mime:"image/bmp", data:"<base64 ≤256 KiB>"}`.
   - The App returns `data:""` on failure rather than `request-failed` (`nativecontrol/session.go:178-195`).
   - The page checks `sourceKey === nativeCaptureTargetKey(target)`:
     - window: `window:<sourceId>:<pid>:<creationTime>`
     - other kinds: `<kind>:<sourceId>`
   - Quirk: the Go side returns `display:<id>` for `picker` targets (`session.go:1200-1205`), so a picker preview
     is rejected by the page.
7. Selection (`HostPage.tsx:1653-1670`): `startSharing({kind:"native", client, target, audio, excludeAudio?, showCaptureBorder, path})`.
   - In the `live` phase it switches source instead (§4.8).

### 4.2 `start-share` (`HostPage.tsx:1369-1380`; `client.ts:344-371`; Go `nativecontrol/wire.go:41-59`, `session.go:196-221, 631-773`)
Request fields:
```
{type:"start-share", shareId:<shareGeneration>, source:<CaptureTarget>, audio:<bool from picker>,
 excludeAudio?:<window target> (only if audio && processAudioExclusion; requires microphoneMixing),
 adapterIndex, encoderIndex, edgeCapacity: 3 (MAX_ENDPOINT_MEDIA_CHILDREN, src/shared/media-copy-accounting.ts:2),
 profile:<QualitySettings>, codec: videoCodecModeRef ("h264" default; "auto"|"h264"|"vp8"),
 microphoneMixing?:true  (iff nativeMedia.microphone || nativeMedia.shareVolume),
 remoteInput?:true       (iff nativeMedia.remoteInput),
 showCaptureBorder?:bool (iff nativeMedia.captureBorderControl),
 reportSourceCancellation?:true (iff nativeMedia.sourceCancellation),
 reportCaptureFailure?:true     (iff nativeMedia.captureFailureDetails)}
```
- App validation (protocol violation):
  - `shareId` must match `^[A-Za-z0-9_-]{8,256}$`; the source must be valid.
  - `edgeCapacity` must be in [1, 20] (`mediaedge.MaxRelayChildren`); codec must be in the enum.
- App validation (operation failure): the profile must be valid; `nativeQualityProfile` requires an integer
  fps ≥ 1, and `maxAudioBitrate` defaults to 128000 (`session.go:775-796`).
- Response (async): `{type:"share-started", shareId, audio:bool, sourceAudio?:bool, codec:"h264"|"vp8"}`.
  The page asserts the same `shareId`.
- On failure: `request-failed` with `code:"source-canceled"` when the user cancelled the OS picker. The page
  maps it to `DOMException NotAllowedError` and returns to the idle phase (`client.ts:854-857`).
- Only one share per control session. A second share gets "native share is unavailable or already active"
  (`session.go:631-636`).
- Client timeout: 8 s, except picker sources (none). The App's capture start timeout is 5 s
  (`nativehost/session.go:25`).
- For picker sources, `activateNativePicker` can fire an Android intent. Ignore it on desktop.

`startNativeShare` order (`HostPage.tsx:1291-1448`):
1. Wait for any previous `stop-share` cleanup and preview queue.
2. Create a `NativeMediaBridge` (allocates a browser `RTCPeerConnection`).
3. Subscribe to native events for this shareId:
   - `audio-state {sourceAudio, microphone, failed}` → audio flags, `set-audio-presence`, UI.
   - `share-ended {failed, failure?}` → `endSharing` (§4.7).
   - `remote-control-state` → out of scope.
4. Send `start-share`.
5. Record `nativeShareGenerationRef = shareId`, `nativeModeRef = true`, and
   `videoCodecRef = started.codec`.
6. If `nativeMedia.remoteInput`: `set-remote-input-peers {shareId, peers}` (remote control; skip).
7. `await bridge.start(started.audio)`. The resulting MediaStream becomes the preview and `streamRef` (§4.3).
   - Any failure in steps 2-7 disposes the share: `stop-share` (or a control discard), close the bridge, and
     rethrow.

### 4.3 Local preview bridge — hidden browser dependency (`src/client/native/media-bridge.ts`)
- **Steps:**
  - `connectionId = createOpaqueId()`.
  - Send `{type:"prepare-local-edge", shareId, connectionId, restart:false}` (no iceServers), answered with
    `{type:"edge-offer", shareId, connectionId, sdp}`.
  - The page uses the response `id` as `offerId` (`client.ts:571-594`; App `nativehost/session.go:325-337`).
  - It sets the remote description, creates an answer (stereo Opus munged), waits for **ICE gathering complete**
    (non-trickle), then sends `{type:"edge-answer", shareId, connectionId, sdp, offerId}`.
  - It waits for `connectionState === "connected"`, a video track, and an audio track if `audio`.
- **Timers:** an 8 s deadline (`BRIDGE_TIMEOUT_MS`). After ready: `disconnected` waits 3 s, and `failed`
  triggers an immediate restart with `prepare-local-edge restart:true`.
- **Coupling to sharing:**
  - Failure after ready calls `endSharing("native.fail.edge")` (`HostPage.tsx:1320-1328`).
  - `watchCaptureEnd` ends sharing when the bridge video track ends (`:1271-1289, 2931`).
  - On dispose, the page sends `close-edge` for that connectionId.
- **Port impact:** the local edge does not count against `edgeCapacity`. mediaedge keeps a separate single
  `localReservation` (`internal/app/mediaedge/source.go:562-591`). The viewer path does not depend on it, so a
  native host can skip it or replace it with its own decoder.

### 4.4 `startSharing` ordering (`HostPage.tsx:2851-3015`)
1. Guard: not already starting/live, and no room mutation in flight. Then `beginRoomMutation("sharing")`.
2. `generation++`, `shareGeneration = createOpaqueId()`, phase `starting`.
3. `startNativeShare` (§4.2-4.3). On error: `setCaptureError`, then phase `idle` (permission/cancel) or `error`.
4. `streamRef = bridge stream`. `details` = `{resolution:null, frameRate:null, hasSourceAudio: nativeSourceAudio}`.
5. Room: `await roomInitializationRef`.
   - If there is no room: `POST /api/rooms`, `writeHostRoom`, `setRoom`.
   - If that fails, all resources are disposed and the phase becomes `error` (401 means the gate).
6. `connectRoomSignal(room, shareGeneration)`. This reuses the existing socket if it already owns
   `roomId+hostToken` (`HostPage.tsx:2629-2634`). Otherwise it creates one (§3.2).
7. `signal.startSharing(shareGeneration, qualitySettingsRef, routePolicyRef)`:
   - Sets `identity.shareGeneration`, removes `roomOnly`, and sends `start-sharing` now if authenticated.
   - Otherwise it is queued and flushed on `authenticated` (`reconcileHostShareState`).
8. `signal.start()` (a no-op if already started).
9. Phase `live` comes from `sharing-started` (§3.5), not from capture.

Server `start-sharing` (`signal/server.go:1444-1512`):
- `FORBIDDEN` if `!roomSession`, the session already has a shareGeneration, or the generation equals the
  retained fence.
- Otherwise it settles the share, connects the host to the route controller, and sends `sharing-started`. It then
  sets the publication, completes authentication in the router, and sends `viewer-presence` and `host-status`
  to viewers. The router then starts preparing routes for waiting viewers (§5).

### 4.5 Pause / resume (`HostPage.tsx:1990-2042`)
1. Native `{type:"pause-share", shareId, paused}` → `share-paused` (App `nativehost/session.go:442-458`).
   Resume requests a keyframe.
2. Then `signal.setSharingPaused(paused)`, which sends `{type:"set-sharing-paused", shareGeneration, paused}`.
   - If resume was not sent (signaling down): send native `pause-share paused:true` again,
     `confirmSharingPaused()`, and show the notice `host.pause.signalRecovering`.
3. Set the flag on all peers (a no-op for native edges). The UI shows `sharingPaused`.
4. Server side: a wrong generation closes the socket with 4001 (`signal/server.go:1377-1398`).

### 4.6 Quality presets → profile
- Presets (`src/client/media/quality.ts:85-102`):
  - `720p30` = `{resolution:{1280,720}, maxFramerate:30, maxBitrate:3_000_000, degradationPreference:"balanced", maxAudioBitrate:128000}`
  - `1080p30` = `DEFAULT_QUALITY_SETTINGS` = `{1920×1080, 30, 5_000_000, "balanced", 128000}` (`protocol.ts:208-214`)
  - `1080p60` = `{1920×1080, 60, 12_000_000, "balanced", 128000}`. This is the default host quality
    (`lib/quality-preference.ts:8`).
- `degradationPreference` is one of `maintain-resolution`, `balanced`, `maintain-framerate`.
  `maxAudioBitrate` is in [6000, 510000], with a UI cap of 320000 (`quality.ts:141`).
- `supportsCaptureProfile` (`quality.ts:72-79`): fps must be an integer within the H.264 L5.2 macroblock
  budget, and bitrate in [100 kbps, limits.maxVideoBitrate].
  - Limits are clamped by `health.nativeMedia.max*` while native is active (`videoCaptureLimits`,
    `HostPage.tsx:3826`).
- **Live change** (`HostPage.tsx:1782-1971`):
  1. Native `{type:"update-share", shareId, profile}` → `{type:"share-updated", shareId}` (no client timeout).
  2. On success, commit the profile.
  3. Save it: `PUT /quality-preference` or localStorage.
  4. Signal `set-quality-settings`.
  5. `peer.updateCaptureProfile` (a no-op for capture-backed native peers, `native-sender-peer.ts:197-207`).
  - Changes made while one is in flight are coalesced to the latest value (`pendingQualityChangeRef`).
- **Not live:** commit and remember only (`:1792-1799`).

### 4.7 Stop / end
- **User stop or cancel:** `endSharing({key:"host.stopNotice"|"host.startCancelled"})` (`HostPage.tsx:4091, 4101`).
  `endSharing(message, notifyServer=true, ..., keepRoomSession=true)` (`:1033-1053`) runs:
  1. `writePreferredRoom`.
  2. `disposeResources` (`:916-989`):
     - `signal.stopSharing(gen)` sends `{type:"stop-sharing", shareGeneration}`. Signaling becomes `roomOnly`
       and keeps the socket.
     - `disposeNativeShare` (`:1672-1700`): dispose the bridge (`close-edge` local), then
       `{type:"stop-share", shareId}` → `share-stopped`. On failure, discard the control client.
       Then release the control connection if it is no longer needed.
     - Dispose all peers: one `close-edge {shareId, connectionId}` each. The App ACKs `edge-closed` even after
       stop (`session.go:515-525`).
     - Clear the prepared child, presence evidence and pause.
  3. Phase `ended`.
- **Native `share-ended`** (`HostPage.tsx:1355-1366`): `endSharing(failed ? "host.shareEnded"/"app/capture/failed" : "host.stopNotice")`.
  - Emitted by the App when the capture ends (`nativecontrol/session.go:1013-1031`):
    `{version:10, type:"share-ended", shareId, failed, failure?:{stage?, hresult?, exitCode}}`.
- **Control socket closed while native** (`handleNativeClientClosed`, `HostPage.tsx:1450-1460`):
  `endSharing("native.fail.controlDisconnected")`.
- **`endRoomSession`** (`:1055-1068`): a terminal stop that also stops signaling (`keepRoomSession=false`).

### 4.8 Source switch while live (native → native, `switchNativeSource`, `HostPage.tsx:3017-3107`)
- Request: `{type:"replace-share-source", shareId, source, audio, excludeAudio?, adapterIndex, encoderIndex, showCaptureBorder?, reportSourceCancellation?}`
  → `{type:"share-source-replaced", shareId}` (async; `client.ts:391-418`; Go `session.go:370-408`).
- The share, edges and shareGeneration are unchanged. Nothing is sent to signaling, except
  `reset-sender-quality` when the video source changed (not just audio).
- An audio-only change on the same target counts as `audioOnly` (`changeNativeSourceAudio`, `:3192-3196`).
- Switching browser→native (`switchToNativeSource`, `:3122-3158`) creates a second App share with a new shareId.
  It is out of scope for an App-only host.

### 4.9 Native events the host consumes (`wire.ts:297-457`; Go `nativecontrol/wire.go:303-396`, `session.go:1080-1109`)
Every event carries `{version:10, type, shareId, ...}`.

| event | fields | page use |
|---|---|---|
| `capture-state` | `state:"starting"\|"active"` | ignored by HostPage |
| `audio-state` | `sourceAudio, microphone, failed` | audio flags, `set-audio-presence`, notice |
| `share-ended` | `failed, failure?` | endSharing |
| `edge-candidate` | `connectionId, candidate:{candidate,sdpMid?,sdpMLineIndex?,usernameFragment?}\|null` | §5.5 |
| `edge-state` | `connectionId, state: new\|connecting\|connected\|disconnected\|failed\|closed` | §5.8 |
| `edge-path` | `connectionId, localType, remoteType (host\|srflx\|prflx\|relay), natTraversalPath: unknown\|ordinary\|predicted` | metrics/UI |
| `edge-quality` | `connectionId, sampleTimestampMs, sampleWindowMs(1000..5000), rtpStatsId, trackIdentifier, state: unknown\|healthy\|degraded, reason: null\|"none"\|"bandwidth", intervalFramesEncoded, framesPerSecond, bitrateKbps, availableOutgoingKbps, width, height` | sender evidence (§5.9) |
| `remote-control-state`, `publication-*` | — | out of scope |

---

## 5. Viewer connection (critical path)

### 5.1 Roles and identifiers
- Each viewer gets its own WebRTC edge on the App side (`mediaedge.Edge`, pion).
  - The page's `NativeSenderPeer` (`src/client/native/native-sender-peer.ts`) and `NativeSenderEdge`
    (`native-sender-edge.ts`) only relay control traffic.
- Edge `connectionId`:
  - Prepare path: equals the **server-chosen** `candidate.connectionId` = `"route_" + opaqueId`
    (`signal/router_pump.go:314`, `router.go:613`).
  - Ordinary path (`startPeer`): `createOpaqueId()` (32 hex). The server **adopts** it into the committed
    direct edge when the offer arrives (`router.go:687-707`). Offers whose `route_`-prefixed id does not match
    are denied.
- The host can serve at most `endpointMediaCopyCapacity` children (Local config = 2, `local.go:119`). The page
  slices `childPeerIds` to that number (`HostPage.tsx:2401-2403`). The server places further viewers under
  other viewers (relay tree).
- The page reports each viewer's peerId as `targetPeerId` and uses `fromPeerId` from the server.
- `native-viewer-peer.ts` (NativeCapableViewerPeer, `receive-offer` / `receive-candidate`) is **viewer-side**.
  The host never uses it.

### 5.2 `route-update` interpretation (`HostPage.tsx:2553-2586`; schema `protocol.ts:1186-1204`)
- Shapes:
  - `{type:"route-update", revision, phase:"prepare", assignment, candidate:{childPeerId, connectionId, transport:"direct"|"sfu", qualityProbe, connectionAttempt?:{current,total}}}`
  - `{type:"route-update", revision, phase:"active", assignment}`
  - `assignment = {upstream:{kind:"none"}, childPeerIds:[peerId…] (unique, ≤20), sfuPublicationGeneration:null}` for the host.
- **Gate.** Every update first goes through `HostSfuRoute.accept` → `MediaRouteTransition.accept`
  (`media/host-sfu-route.ts:87-151`, `media/route-transition.ts:51-82`):
  - `revision < last` → `"stale"`, which is ignored.
  - Same revision:
    - It is `"stale"` if the assignment differs, if the prepare candidate differs, or if a `prepare` arrives after
      `active`.
    - It is `"duplicate"` if the phase is unchanged.
  - Otherwise `"accepted"`.
  - On `phase:"active"` and not duplicate: `reconcileChildren(assignment, revision)` (§5.4).
- **Prepare:**
  - Duplicates are ignored.
  - `candidate.transport === "direct"` → `prepareHostChild` (§5.3). If it returns false:
    `route-failed {revision, phase:"prepare", connectionId: candidate.connectionId}`.
  - `transport:"sfu"` → discard the prepared child (SFU; never happens in the App).
- **Active:** set `activeRouteRevision = revision`.
- The same reconcile runs for `authenticated.routeAssignment` and `sharing-started.routeAssignment` through
  `resyncAuthoritative`. If that revision is stale, the page first resets and reconciles to an empty child list
  (`host-sfu-route.ts:161-194`).

Server side (`signal/router_pump.go:422-479`, `router.go:1325-1374`):
- One route operation at a time per room. For a direct candidate under the host, `prepare` is sent to the child
  viewer first, then to the host.
- `active` is broadcast to host + all viewers when the controller revision changes:
  - after viewer `route-ready` (first decoded frame, `router.go:810-860`),
  - after `route-transport-connected` progress (`:876-907`),
  - after a failure (`:937-`).
- Operation timeout: 20 s (`defaultRouteOperationTimeoutMs`, `router.go:29`). Up to 4 NAT connection attempts
  per candidate (`route/controller.go:16`).

### 5.3 Prepared child (`prepareHostChild`, `HostPage.tsx:2143-2242`; `media/host-provisional-child.ts`)
- **Preconditions:** the share is current, native mode is on (or a stream exists), `iceConfigRef` and signaling
  are present.
- **`validPlannedHostChildren`** (`host-provisional-child.ts:305-326`) checks all of:
  - `candidate.transport === "direct"`.
  - `candidate.childPeerId ∈ assignment.childPeerIds`, with no duplicates.
  - The planned list contains every currently active child and at most one new one.
  - `childPeerIds.length (+1 if sfuPublicationGeneration) ≤ endpointMediaCopyCapacity`.
  - On failure: discard and return false, so the page sends `route-failed prepare`.
- **Idempotence:** the same revision, candidate and plan returns the current state.
- If the active peer for that child already has `connectionId === candidate.connectionId`: discard, return false.
- **Peer factory (native):**
  - Normally: `new NativeSenderPeer(childPeerId, candidate.connectionId, nativeShareId, iceConfig, routePolicy.natPrediction, client, events, codec, undefined, retry = (connectionAttempt?.current ?? 1) > 1)`.
  - **Hidden browser dependency:** if `candidate.qualityProbe` and the child's current peer is a
    `NativeSenderPeer`, the page builds a **browser `HostPeer`**. It re-encodes the local preview stream
    (`shouldUseBrowserQualityCandidate`, `native-sender-peer.ts:231-236`; `HostPage.tsx:2196-2213`).
  - `qualityProbe` is true only for `quality-convergence` demands (`router_pump.go:553`).
  - A native port should use the native edge here too, or answer `route-failed prepare`.
- **Events:**
  - Snapshot `connectionState === "connected"` for the candidate → once
    `route-transport-connected {revision, connectionId}` (`host-provisional-child.ts:131-143`;
    `HostPage.tsx:2190-2192`).
  - `failed` → dispose (`close-edge`) and `route-failed {revision, phase:"prepare", connectionId}`
    (`:288-302`, `HostPage.tsx:2187-2189, 2436-2449`).
  - Signals go out only while this peer is the signaling owner and `payload.connectionId === candidate.connectionId`.
- **Activation:** on `route-update active` (`activatePreparedHostChild`, `HostPage.tsx:2244-2265`;
  `host-provisional-child.ts:236-264`), the prepared peer is **promoted** into `peersRef[childPeerId]` when:
  - the revision equals the prepared revision,
  - the child is in the new `childPeerIds`,
  - and either the child was not active or the replaced connection is still current.
  - On promotion the old peer for that child is disposed. Otherwise the prepared peer is discarded.
- `reconcileHostChildren` runs right after activation.

### 5.4 Reconcile children (`reconcileHostChildren`, `HostPage.tsx:2397-2417`; `webrtc/media-assignment.ts:24-45`)
- `activeHostChildPeerIds = unique(childPeerIds).slice(0, endpointMediaCopyCapacity)`.
- For each existing peer not in the set, `removePeer`: `peer.dispose()` sends `close-edge` and retires evidence
  (`HostPage.tsx:2092-2104`).
- For each child in the set, `startPeer(peerId)` (`:2271-2355`). It returns early if a peer already exists, so
  promoted prepared peers are kept. Otherwise it creates
  `new NativeSenderPeer(peerId, createOpaqueId(), nativeShareId, iceConfig, natPrediction, client, events, codec)`
  with no retry/punch, and calls `start()`.
  - `start()` false or throws → `route-failed {revision: activeRouteRevision, phase:"active", connectionId}`
    plus `removePeer`.

### 5.5 Offer creation and outbound ICE (`native-sender-edge.ts:87-131, 165-229`)
1. Subscribe to native events for this `(shareId, connectionId)`.
2. Optional punch (§5.10). Only when `retry && natPrediction && survey has 3 URLs`.
3. Prepare edge request:
   `{type:"prepare-edge", shareId, connectionId, punched?:true, iceServers:[{urls:[...]}...]}`
   → `{type:"edge-offer", shareId, connectionId, sdp(≤48 KiB)}` (`client.ts:420-447`; Go
   `nativecontrol/session.go:798-829`, `nativehost/session.go:339-419`).
   - `iceServers` = `iceServersWithNatPrediction(iceConfig.iceServers, predictionEnabled, natPredictionStunUrls)`
     with `urls` normalized to arrays (`webrtc/nat-prediction.ts:135-163`). Up to 8 servers × 8 URLs, STUN only.
   - The App creates a pion PeerConnection with a send-only video track (H.264 or VP8) and an Opus track when
     the share has audio.
   - If remote input is on, it also opens a `remoteinput` data channel. It then creates a trickle offer.
4. Send signaling
   `{type:"signal", targetPeerId: childPeerId, payload:{kind:"description", connectionId, description:{type:"offer", sdp}, punched?:true}}`.
   If `send` returns false (signaling down), the edge is disposed and start returns false.
5. `offerSent = true`, then flush queued candidates.
- **Candidates (`edge-candidate` event):**
  - Default (natPrediction off, i.e. Local mode): each one is forwarded as
    `{type:"signal", targetPeerId, payload:{kind:"candidate", connectionId, candidate:<obj>|null}}`.
    `null` means end-of-candidates.
  - Before the offer is sent, candidates are queued (max 64, `MAX_PENDING_CANDIDATES`). Overflow means
    dispose plus `failed`.
  - With natPrediction on (link mode with 3 survey URLs), "NAT batching" applies (`nat-prediction.ts:310-359`).
    It **does not delay**:
    - Every candidate is forwarded immediately.
    - Srflx candidates from survey servers feed the predictor. The App names a survey candidate's foundation
      `ns<k>`, where `k` is the 1-based index of the STUN URL in the flattened iceServers list
      (`isNativeNatSurveyCandidate`, `:11-25`).
    - Once ≥3 distinct srflx ports with a constant step exist in one group, or on `null` completion, up to 8
      synthetic candidates are appended. They are `candidate:sp1..4` (+step) and `candidate:sm1..4` (−step)
      (`predictSrflxCandidates`, `:276-303`).
    - Then the `null` end marker is sent.
- Server authorization of host→child signals (`signal/server.go:1695-1778`, `router.go:617-707`):
  - Signals for the operation's `current.ConnectionID` are always authorized ("probe").
  - On a committed edge, an offer with a different non-`route_` id is adopted.
  - Unauthorized signals are **silently dropped**.
  - A non-connected target yields `error PEER_NOT_FOUND`, unless the pair is assigned.

### 5.6 Inbound signals (`HostPage.tsx:2602-2613`)
`{type:"signal", fromPeerId, payload}` is dispatched as follows:
- First to the prepared child, if `fromPeerId === candidate.childPeerId && payload.connectionId === candidate.connectionId`
  (`host-provisional-child.ts:189-207`).
- Otherwise to `peersRef[fromPeerId]`, then `NativeSenderEdge.acceptSignal`. It ignores a mismatched
  `connectionId` (`native-sender-edge.ts:133-144`).
- Payload kinds (`client.ts:665-686`):
  - `description` with `type:"answer"` → `{type:"edge-answer", shareId, connectionId, sdp}` → `edge-answer-accepted`.
    A viewer `offer` is ignored.
  - `candidate` → `{type:"edge-candidate", shareId, connectionId, candidate:<obj>|null}` → `edge-candidate-accepted`.
    For an unknown or retired edge the App ACKs without error (`nativehost/session.go:429-440`).
  - `punch` → the punch state machine (§5.10).
- If `edge-answer`/`edge-candidate` is rejected, the peer snapshot becomes `failed` with
  `native.fail.signaling` (`native-sender-peer.ts:157-171`). That leads to `route-failed` (§5.8).

### 5.7 `restart-request` (`HostPage.tsx:2614-2626, 2357-2395`)
- Message: `{type:"restart-request", fromPeerId, connectionId, rebuild}`. Only viewers send it. The server checks
  that the sender is the assigned child (`signal/server.go:1780-1800`).
- Ignored unless the child is assigned and `peer.connectionId === connectionId`. With `rebuild:true` and no
  peer, a fresh peer is started anyway.
- `NativeSenderPeer.restartIce()` always returns false. So both `rebuild:true` and `rebuild:false` do the same:
  `removePeer` (`close-edge`), then `startPeer` with a **new** opaque connectionId (adopted by the server, §5.1).

### 5.8 Edge state handling (`native-sender-peer.ts:61-100`; `HostPage.tsx:2132-2141`)
- `edge-state` maps to the snapshot `connectionState`. ICE state is derived: connected, failed, closed, otherwise
  checking.
- For an **active** peer, `failed` or `closed` triggers:
  - `route-failed {revision: activeRouteRevision, phase:"active", connectionId}`, only if the child is still
    assigned (`reportHostChildFailure`, `:2419-2434`),
  - `removePeer`, which sends `close-edge`.
- Not handled locally: `disconnected` (pion may recover; the viewer drives restart requests).
- For a **prepared** peer, `failed` means `route-failed prepare` (§5.3).
- `share-ended` sets all edges to `closed` or `failed`. Normally endSharing has already torn them down.
- Every other update refreshes the UI snapshot and calls `reportSenderQuality`.
- Close request: `{type:"close-edge", shareId, connectionId}` → `edge-closed` (`client.ts:688-694`). It is fire
  and forget on dispose.

### 5.9 `sender-quality-evidence` (optional for viewers to work)
- Built from each peer snapshot (`media/sender-quality-evidence.ts:98-157`). Sent only when
  `routePolicy.topologyOptimization && document.visibilityState === "visible"` (`HostPage.tsx:2116-2130`).
- Shape (`protocol.ts:799-844`; Go `protocol/client.go:129-139`):
  `{type:"sender-quality-evidence", childPeerId, connectionId, rtpStatsId|null, trackIdentifier|null, sampleTimestampMs|null, routeRevision, state:"unknown"|"healthy"|"degraded", diagnostics:{natTraversalPath:"unknown"|"ordinary"|"predicted", reason:null|"none"|"bandwidth"|"cpu", framesPerSecond, bitrateKbps, captureFramesPerSecond?, mediaSourceFramesPerSecond?, width?, height?, availableOutgoingKbps?, rttMs?, packetLossPercent?}}`.
- Rules:
  - A known state requires `rtpStatsId`, `trackIdentifier` and `sampleTimestampMs`.
  - Reason must match the state: healthy ↔ "none", degraded ↔ bandwidth|cpu, unknown ↔ null.
  - The first report per connection after `invalidateSenderQualityEvidence()` is `unknown`, and `unknown` is sent
    only once per connection.
  - `sampleTimestampMs` must increase.
- Server: rate-limited per session to (20+1)×2 messages per 2 s (`signal/server.go:1675-1689`). Only feeds
  quality convergence.
- The route controller treats the host as always usable (`route/candidates.go:393-396`). **Skipping it does not
  block viewers.**

### 5.10 Punch (link mode retries only; `native/native-punch.ts`)
- **Preconditions:** `retry` (connectionAttempt.current > 1), natPrediction on, and `punchIceConfig` finds
  exactly 3 survey URLs. The page then uses `{iceServers:[{urls:[3 URLs]}]}`. 3.5 s preparation timeout.
- **Sequence:**
  1. Generate a 32-byte hex secret.
  2. App: `{type:"prepare-punch", shareId, connectionId, secret, iceServers}` →
     `{type:"punch-prepared", prediction:{address(ipv4), port(1024..65535), step(0..16)}|null}`.
  3. Signal punch `{kind:"punch", connectionId, punch:{phase:"prepare", secret}}`.
  4. Viewer replies `{phase:"ready", prediction?}`. Without a prediction, the punch fails.
  5. Signal punch `{phase:"start", prediction:<local>}`, and App
     `{type:"run-punch", shareId, connectionId, prediction:<remote>}` → `{type:"punch-complete", connected}`.
  6. Viewer sends `{phase:"complete", connected}`.
  7. If both sides are true: `punched = true`, then `prepare-edge punched:true` and an offer with `punched:true`.
     Otherwise the page calls `{type:"close-punch", shareId, connectionId}` → `punch-closed` and continues with
     ordinary ICE.
- The server allows punch signals only for the current operation with `connectionAttempt.current > 1`
  (`router.go:644-654`). **Skippable.**

### 5.11 Reconnect behaviour with live edges
- On reauthentication, peers that are not connected are removed (their offer may be lost). Connected peers stay.
- `resyncAuthoritative` then reconciles to the server's assignment, creating new peers where needed.
- Native edges keep flowing during a signaling outage. Trickle that fails while signaling is down is dropped
  (`native-sender-edge.ts:226-228`).

---

## 6. Status shown in the minimal room screen

| UI element | source of truth | code |
|---|---|---|
| Room code chip | `room.roomId` (state `room`, persisted §2.6); placeholder + Create if null | `HostPage.tsx:4271-4288` |
| Admission badge | `room.codeEntryPolicy` (from create response / `authenticated` / access API) + `viewerPasswordEnabled` | `:4278-4283` |
| Invite link field | `includeInviteCredential ? room.inviteUrl : room.canonicalUrl`; Copy disabled when null/blocked/mutating | `:563-565, 4400-4460` |
| Rotate / revoke | access API (§2.3) | `:4414-4430` |
| Connection LED (header) | `signalStatus` → `SIGNAL_VISUALS`: connected = live/green, connecting = busy, reconnecting = warn, offline = off | `:3811-3817, 3845-3849`; `ui/media-status.ts:97-102` |
| TV state/activity | `phase` (`idle`, `starting`, `live`, `ended`, `error`) + `sharingPaused`; `idle && room` shows "room ready" | `ui/media-status.ts:87-95, 104-130` |
| Stage overlays | picker open; switching; paused; `starting`; preview suspended | `HostPage.tsx:3993-4020` |
| Buttons | Start (idle/ended/error), Switch + Pause/Resume + Stop (`live`), Cancel (`starting`) | `:3930-3947, 4022-4103` |
| Viewer count / couch | `viewers = labelParticipantSnapshot(participantPresence).viewers` (role viewer from `viewer-presence`); count = `viewers.length` | `:665-668, 3717-3726` |
| Per-viewer status | `deriveParticipantStatus(viewer, phase==="live", evidence)`: `mediaReady:true` = connected; `upstream.kind none` = "routing"; peer/sfu without mediaReady = "connecting" | `ui/media-status.ts:33-56` |
| Host name | `displayName` (local) / `hostPresence.displayName` | `:3856` |
| Notices | `noticeValue` (operation pill or television status) | `:520-562, 4105-4118` |

Phase transitions:
- `idle` → `starting` on startSharing.
- `starting` → `live` on `sharing-started`, or on `authenticated` with media active.
- `starting` → `idle` on capture cancel or permission failure.
- `starting` → `error` on other failures.
- `live` → `ended` on stop, `share-ended` or control loss.
- any → `error`/`ended` on `endRoomSession`.

---

## 7. Minimum viable native host checklist

Goal: a browser or Android TV viewer opening `<PublicBaseURL>/r/<code>#v=<grant>` sees the natively
captured screen.

**Must implement:**
1. **Site access.**
   - If the launcher password is non-empty: `POST http://127.0.0.1:8787/api/site-access` with
     `Origin: http://127.0.0.1:8787` and body `{"password":…}`.
   - Keep the `piik-site-access` cookie.
   - Send `Origin` + `Cookie` on every later API call and on the WS upgrade.
   - Renew with `GET /api/site-access` at least every 24 h; the page does it hourly.
2. **Room.**
   - `POST /api/rooms {"codeEntryPolicy":"open"[,"preferredRoomId":last]}`.
   - Persist `{roomId, hostToken, inviteUrl, grant}` and show `inviteUrl`.
   - On restart, reuse the persisted room via signaling. If `INVALID_TOKEN` or `ROOM_NOT_FOUND`, create anew.
3. **Signaling.**
   - Dial `ws://127.0.0.1:8787/signal` with Origin + Cookie.
   - Within 5 s send `authenticate` (§3.2 example, `roomSession:true, roomOnly:true`). Wait for `authenticated`;
     keep `peerId`, `iceConfig` and `endpointMediaCopyCapacity`.
   - Respond to WebSocket pings. Reconnect with 0.5-8 s backoff, except after 4001, 4003, 1008 or 4004.
   - On reconnect while sharing, authenticate with `shareGeneration` and without `roomOnly`.
     - If the server returns `shareGeneration:null`, send `start-sharing` again.
4. **Native control (in-process or WS).**
   - `capture-options` + `list-sources` → pick target and adapter/encoder (first adapter with
     `hardwareH264[0]`).
   - `start-share {shareId:G, source, audio, adapterIndex, encoderIndex, edgeCapacity:3, profile:1080p60, codec:"h264"}`.
     Add `microphoneMixing:true` only if the capability is present; omit other optional flags.
5. **Publish:** `start-sharing {shareGeneration:G, qualitySettings, routePolicy:{peerOnly:true, topologyOptimization:true, natPrediction:<cap>}}`.
   Wait for `sharing-started`.
6. **Route handling** (dedupe by revision as in §5.2):
   - `prepare` + `transport:"direct"`:
     - `prepare-edge {shareId:G, connectionId: candidate.connectionId, iceServers}`.
     - Send the offer as `signal {targetPeerId: candidate.childPeerId, payload:{kind:"description", connectionId, description:{type:"offer", sdp}}}`.
     - Relay App `edge-candidate` events as `kind:"candidate"`, including the final `null`.
     - Send `route-transport-connected` when the edge is connected (recommended).
     - Send `route-failed prepare` on failure or prepare error.
   - `active`:
     - Close edges of peers no longer in `childPeerIds`.
     - For new children without an edge, `prepare-edge` with a fresh opaque id and send the offer.
   - Apply the same reconcile to `authenticated.routeAssignment` and `sharing-started.routeAssignment`.
7. **Inbound `signal`:** route by `(fromPeerId, connectionId)`. `answer` → `edge-answer`; `candidate` →
   `edge-candidate`.
8. **Failure handling:**
   - `edge-state failed|closed` on an assigned child → `route-failed {revision: lastActive, phase:"active", connectionId}`
     plus `close-edge`.
   - `restart-request` → `close-edge` plus a new edge with a new connectionId.
9. **Stop:** `stop-sharing {shareGeneration:G}` and `stop-share {shareId:G}`. Handle App `share-ended` the same way.
10. **UI:** room code, invite link, LED (signal status), phase, viewer count from `viewer-presence`.

**Safe to skip** (viewers still get video):
- Local preview bridge (`prepare-local-edge`).
- `sender-quality-evidence` and `reset-sender-quality`.
- Punch (`*-punch`).
- NAT-prediction synthetic candidates (Local mode never enables them; link mode just loses an optimization).
- `signaling-challenge` watchdog (server pings suffice; it is only a faster dead-socket detector).
- `set-audio-presence` (viewers' audio-available icon only).
- `set-co-play-presence`, `subscribe-remote-input`, `subscribe-room-interactions`, `subscribe-room-voice`.
- Display name changes.
- Access-policy and replacement APIs.
- Quality-preference sync (`/quality-preference`).
- Presentation POST and Web Locks.
- The quality-probe browser path: use a native edge or `route-failed prepare`.
- Pause (but implement `pause-sharing-source` → `pause-share paused:true` if you want parity).

**Gotchas for the port:**
- Both the server and the App decode strictly:
  - The server closes with 1008 on any extra or unknown field and never auto-reconnects.
  - The App treats unknown fields in control requests as a protocol violation and closes control, which stops
    the share.
- `connectionId`, `shareId` and request ids must match `^[A-Za-z0-9_-]{8,…}$`. Use 32-hex opaque ids.
  Never reuse a `route_…` id for a self-started edge.
- One control session can own only one share. Closing the control session kills it.
- `endpointMediaCopyCapacity` (2 in Local mode) bounds direct children. More viewers are relayed by viewers,
  which is server policy.
