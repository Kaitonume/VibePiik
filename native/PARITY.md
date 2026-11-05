# VibePiik native host — feature parity checklist

Web sources: `src/client/pages/HostPage.tsx`, `AppLauncherPage.tsx`, `ViewerPage.tsx`, `JoinPage.tsx`,
`src/client/components/living/**`, `src/client/room-voice/**`, `src/client/ui/*`, `src/client/locales/en.ts`.
Native: `internal/app/nativeui/*.go` (as of 2026-11-05).

Status: **done** (same user-visible result), **partial** (what is missing is named), **missing**, **n/a**
(does not apply to the Windows App host). Effort: **S** < ½ day, **M** 1–3 days, **L** > 3 days.
IDs (e.g. `RS-4`) are used by the priority list at the end.

---

## 1. Header / top bar (`Header.tsx`, `AppConnectionButton.tsx`, `LanguageControl.tsx`, `PiikTvSendButton.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| HD-1 | Brand mark (`brand.home`) | Mascot at top left; on the host page it is the current page (no link) | done | – | `room.go header` |
| HD-2 | Server connection LED strip (`LedStrip`, `state.signal.*`) | 3 dots + label: connected / connecting / recovering / not connected, tone live/busy/warn/bad | done (batch 5): Recovering state | S | host.go sets `signal` |
| HD-3 | LED tooltip with state comic | Click/hover shows comic + label | done (batch 10): header LEDs show the state comic, click pins it | M | needs tooltip + comics (§13) |
| HD-4 | PIIK TV send button (`send.label`, `send.sentLabel`) | Sends invite to TVs on LAN; busy spinner, check when sent, alert on failure, auto-reset 5 s | done | – | `Room.sendTV`; failure text replaces the caption instead of a tooltip |
| HD-5 | PIIK TV button tooltip (`send.hint`, `send.noRoom`, result detail) | Explains action / why disabled / which TVs got it | done (batch 6) | S | needs tooltip |
| HD-6 | App connection button (`client.connect.*`) | Shows App state (ready / update available), popover with version, release notice, open App page | done (batch 9) | S–M | in the App's own room the App is always connected; at least show version + update notice |
| HD-7 | Language switch 中 / EN / ✦ (`mode.language`, `mode.vis`) | Segmented control, persisted | done | – | persisted in `%LOCALAPPDATA%\VibePiik\ui.json` |
| HD-8 | More languages menu (`mode.more`) | Popover for community locales | n/a | – | no extra locales registered today |
| HD-9 | Theme toggle (`theme.dark/light(.short)`) | Sun/moon button, persisted | done | – | also sets the dark title bar |
| HD-10 | Help menu button (`help.title`, `help.hint`) | Popover: Piik Web version, troubleshooting link, diagnostics | done (batch 9) | M | see §8 |
| HD-11 | Compact header overflow (`header.more`) | ≤600 px header width folds controls into a "More options" popover | done (batch 9) | S | low value: native min width 720 dp |
| HD-12 | Launcher header debug button (`client.launch.debug`, `debugHint`) | CPU toggle, pressed when on, disabled while starting/running | done | – | no tooltip |
| HD-13 | Debug enable failure (`client.launch.debugFailed`) | Red pill when enabling fails | done (batch 9) | S | error ignored |
| HD-14 | Window/document title (`useDocumentTitle`, title frames) | `Piik | done (batch 9) | missing: fixed "VibePiik" | S | helps Alt-Tab/taskbar; title frames in en.ts/zh.ts/visual.ts |

## 2. Launcher page (`AppLauncherPage.tsx`, `LauncherForm.tsx`, `SiteAddressEntry.tsx`, `PiikTvLinkCard.tsx`, `AppReleaseNotice.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| LA-1 | Brand heading "VibePiik. App" | Mascot + wordmark | done | – | |
| LA-2 | Welcome line (`WelcomeLine`, playful.welcome) | Random line with ✦, symbols in visual mode | done (batch 9) | S | |
| LA-3 | Loading / starting state (`common.loading`, `client.launch.starting`) | Breathing brand loader + label, polls every 750 ms | done | – | |
| LA-4 | State load failure (`client.launch.loadFailed`, `common.refresh`) | Red pill + Refresh button | done (batch 1): Retry | S | `reload()` has no error UI/retry |
| LA-5 | Refresh state on window focus | Re-reads launcher state when the window regains focus | done (batch 6) | S | |
| LA-6 | Release update notice (`client.update.*`) | Update / major update / different build / official release link + AtomGit mirror | done (batch 10): GitHub/AtomGit release check, notice in launcher and Help, App button arrow | M | needs `useReleaseUpdate` logic (GitHub release check) |
| LA-7 | Standby card (`client.connect.standby`) | "App is ready" with check | done | – | |
| LA-8 | Connect-a-site guide (`client.connect.guide`, `standbyHint`) | Disclosure with steps (comic in ✦ mode) | done | – | no comic |
| LA-9 | Open a site (`client.connect.addSite`, `siteAddress`, `continue`, `invalid`) | "+" opens address entry → approval page (Allow and open) | done (batch 1) | M | |
| LA-10 | Site approval page (`client.connect.approveTitle/approveHint/approve/approved/return/close`) | Shown for `?site=` or manual entry; allow access, close | done (batch 10): manual entry approves with "Open on start"; ?site= stays web | M | `?site=` is opened in the user's browser by the web page, so still web; manual entry (LA-9) needs it natively |
| LA-11 | Approved sites list (`client.connect.remove`) | Each site is a link opening the site with App launch params | done (batch 1) | S | open via `HostEnv.OpenExternal` |
| LA-12 | Open site when App starts (★, `client.connect.openOnStart`) | Toggle per site, persisted via `/startup-site` | done (batch 9) | S | API exists (`POST /startup-site`) |
| LA-13 | Revoke site access (✕) | Removes site via `/site {allowed:false}` | done (batch 1) | S | |
| LA-14 | Site save errors (`client.connect.changed`, `saveFailed`) | Alert text | done (batch 9) | S | with LA-12/13 |
| LA-15 | Section label "Create a room on this device" (`client.launch.room`) | Caption above mode cards | done | – | |
| LA-16 | Mode cards Public invite / Local room (`client.launch.link/local(+Hint)`) | Radio tiles, arrow-key navigation, tooltip comic | done (batch 10): arrow keys, comic tooltip, focus ring | S | |
| LA-17 | Site passphrase disclosure (`client.launch.accessSettings`, `accessSet`, `localAccess`, `localAccessHint`) | Collapsible password field, "Set" marker | done | – | |
| LA-18 | LAN address chooser (`client.launch.lanAddress`, `chooseAddress`, `lanHint`) | Select shown when ≠1 address or none chosen; opens automatically if none | done | – | radio list instead of select |
| LA-19 | No LAN address warning (`client.launch.lanUnavailable`) | Warning pill | done | – | plain text |
| LA-20 | Local HTTPS note (`client.launch.localHTTPS`) | Shown for local mode | done | – | |
| LA-21 | Open Piik / Retry primary button | Submits launch; disabled when local mode lacks LAN address | done | – | |
| LA-22 | Running room shortcut (`client.launch.go` with target) | Shows only "Open Piik" when a room already runs | done | – | |
| LA-23 | Launch error block (`client.launch.error`, `failure.*`, `retryHint`, `errorDetails`) | Pill, mapped reason, retry hint, collapsible technical details | done (batch 9) | S | |
| LA-24 | Failure reported by background state (`state.detail/reason`) | Shows launch error found while polling | done (batch 9) | S | native ignores `LauncherState.Detail` |
| LA-25 | 409 conflict on launch | Reloads state (another launch already running) | done (batch 10): 409 rereads the App state | S | |
| LA-26 | Launch with debug flag | Sends `debug:true` when diagnostics are on | done (batch 10): debug:true sent when diagnostics are on | S | verify App behaviour |
| LA-27 | PIIK TV link card (`card.title/placeholder/button/hint/invalid*`, `send.*`) | Paste any room link, Enter sends, result message 8 s | done | – | |
| LA-28 | Footer GitHub link | Opens repo | done (batch 1) | S | |
| LA-29 | Footer version (`client.launch.version`) + "· VibePiik" | Installed version, dev builds show short revision | done (batch 6) | S | |

## 3. Room scene: TV, entry, local preview, overlays, playback controls (`HostPage.tsx`, `Stage.tsx`, `PlaybackControls.tsx`, `PlaybackSettings.tsx`, `StatusIndicator.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| RS-1 | TV frame + chin (`StageTv`) | 16:9 screen, chin with status | done | – | `layoutTV` |
| RS-2 | Chin status indicator (`StatusIndicator`, `shareHelp.open`) | Coloured light + label; clickable when failed (opens §8 help) | done (batch 7): status light tooltip with the state comic | S | |
| RS-3 | Welcome line on idle TV | Rotating playful line | done | – | not hidden in ✦ mode |
| RS-4 | Start sharing big button (`host.start`) | Round cast button, ripple; disabled while room mutates | done | – | no disabled state while room creation runs |
| RS-5 | Join a room big button (`host.join`, `host.join.hide`) | Toggles inline room-code entry on the TV | decided: opens the web viewer in the browser (with a note under the button); a native viewer is postponed | M | needs RoomCodeInput (VI-4) natively or a confirm |
| RS-6 | Inline room code entry (`join.hint`, `join.field`, `join.invalid`, `join.submit`) | 4-digit input, shake + error on bad code, go arrow | decided: room code entry stays on the web /join page | M | shared with viewer Join (VI-4) |
| RS-7 | Idle notice on TV (`host.*` notices) | Status notices on the TV chin / notices bar | done (batch 9) | S | see §7 |
| RS-8 | Starting overlay (`host.starting`) | Static noise + "Connecting" comic overlay | done (batch 5): starting comic | S | |
| RS-9 | Cancel while starting (`host.cancelStart`, `host.startCancelled`) | Danger button in share controls | done (batch 1) | S | startShare has no cancel path; needs ctx cancel + stop-share |
| RS-10 | Live local preview of window shares (`host.preview`) | `<video>` of the outgoing stream | done | – | DWM thumbnail (`preview_windows.go`); native extra: zero decode cost |
| RS-11 | Live local preview of screen (display) shares | Same video for screens | done (batch 1): still preview refreshed every 3 s | M | DWM thumbnail of the monitor (DwmRegisterThumbnail on desktop window) or decoded loopback stream |
| RS-12 | Preview hidden behind floating panels / when scrolled | n/a (DOM layering) | done | – | native-only clipping logic |
| RS-13 | Paused overlay (`host.pauseNotice`) | Dim + pause comic "Audio and video sharing paused" | done (batch 5): paused comic | S | |
| RS-14 | Switching-source overlay (`host.switchingSource`) | Dim + refresh comic while switching | done (batch 1) | S | |
| RS-15 | Local preview paused overlay (`host.localPreviewPaused`) | Shown when preview auto-paused | n/a | – | DWM preview never pauses |
| RS-16 | Preview play/pause on this device (`playback.play/pause`) | Pause local preview, sharing continues | done (batch 6): pause/play of the local preview on the TV chin (sharing continues) | S | could hide DWM thumbnail |
| RS-17 | Preview auto-pause setting (`host.previewAutoPause(+Hint)`) | Pause preview when tab/window hidden, persisted | done (batch 6): auto-pause while VibePiik is in the background (setting, default on) | S | low value natively |
| RS-18 | Shared sound mute for all viewers (`host.shareVolume.mute/unmute`) | Speaker button in preview controls | done (batch 1) | M | App control `set-share-volume` exists (`nativeMedia.shareVolume`) |
| RS-19 | Shared sound volume 0–200 % (`host.shareVolume`, `.unavailable`) | Slider + % readout, affects all viewers | done (batch 1) | M | same command; important for TV |
| RS-20 | No-audio indicator (`playback.noAudio`, `host.noAudio`) | Speaker-off button + "Source sound is not being shared" pill | done (batch 6): no-sound mark on the TV chin | S | |
| RS-21 | Picture fit cycle Fit/Fill/Stretch (`playback.fit.*`, `playback.fitCycle`) | Button, persisted (`pictureFit`) | done (batch 6): Fit / Fill / Stretch cycle (live thumbnail and stills) | S | DWM thumbnail supports dest rect cropping |
| RS-22 | Rotate picture 90° (`playback.rotation(+Hint)`) | Button, persisted | n/a: the live thumbnail (DWM) cannot rotate; stills only would be inconsistent | M | DWM thumbnail cannot rotate; needs decoded frames |
| RS-23 | Theater mode (`viewer.theater(.exit)`) | Enlarged stage, page chrome reduced | done (batch 7): theater button on the TV chin (TV takes the full width) | M | |
| RS-24 | Fullscreen preview (`playback.fullscreen*`) | Fullscreen video, double-click gesture | done (batch 7): fullscreen picture only; Esc, F11 or double click to leave | M | Gio window fullscreen + DWM dest rect |
| RS-25 | Picture in picture (`playback.pip*`) | Floating PiP video | done (batch 7): native picture in picture: always-on-top window with the live thumbnail of the shared window, drag, resize, double click closes, hidden from capture; not for whole screens | L | needs separate always-on-top window |
| RS-26 | Playback controls auto-hide + reveal (`playback.showControls`) | Bar hides while playing, chin arrow reveals | n/a: controls sit on the chin, never over the picture | S | with RS-16..24 |
| RS-27 | Playback "More" settings popover (`playback.more`, `viewControls`, `windowControls`) | Picture options, preview, on-screen chat settings | n/a: controls sit on the chin, never over the picture | M | |
| RS-28 | On-screen chat (danmaku) over preview (`RoomChatOverlay`, `playback.overlay`) | Messages fly across the picture, per-lane | done (batch 4): scrolling chat over the TV picture in a click-through layered window above the live preview (3 lanes, newest per lane, 6 s / speed) | L | DWM thumbnail is composited above Gio: overlay needs a layered window or decoded preview |
| RS-29 | Space toggles preview playback | Keyboard on focused video | done (batch 6): Space | S | |
| RS-30 | Static noise / StageOverlay comics | Animated overlay scenes for start/switch/pause | done (batch 10): static noise while starting + state comics | M | §13 |

## 4. Share controls toolbar (`HostShareControls`, HostPage)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| SC-1 | Switch source (`host.switchSource`) | Opens picker while live | done | – | `replace-share-source` |
| SC-2 | Switching busy state (`host.switching`) | Caption "Choosing", disabled during switch/quality change | done (batch 1) | S | |
| SC-3 | Pause / Resume sharing (`host.pause/resume`) | Toggle; disabled while switching | done | – | no busy/disabled state |
| SC-4 | Stop sharing (`host.stop`, `host.stopNotice`) | Danger button, "Sharing stopped" notice | done (batch 5): "Sharing stopped" with the share-ended comic | S | |
| SC-5 | Sharing settings toggle (`host.settings.button`, `host.advanced(.hide)`) | Opens settings door, busy spinner while applying | done (batch 10): spinner while settings apply | S | |
| SC-6 | Remote control trigger (`remote.title`) | Toolbar button, only with App remote input | done | – | see §12 |
| SC-7 | Source switch result notices (`host.notice.sourceSwitch.ok/partial`) | Pill after switch | done (batch 5): "Share source switched" (not for sound-only changes) | S | |
| SC-8 | Stage notices bar (no-audio, SFU warnings, operation notice with dismiss) | Pills under toolbar, dismissable (`notice.dismiss`) | done (batch 9) | M | §7 |

## 5. Source picker (`CaptureSourcePicker.tsx`, `CaptureSourceCard.tsx`, `CameraSources.tsx`, `AudioExclusionSelect.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| SP-1 | Picker covers the TV (`host.sourcePicker.title`) | Dialog over stage, keyboard loop | done | – | |
| SP-2 | Apps / Windows tab (`tab.window`) | Window list | done | – | |
| SP-3 | Screens tab (`tab.display`) | Monitor list | done | – | |
| SP-4 | Camera tab (`tab.camera`, `host.sourcePicker.camera`, `host.camera.denied/unavailable`) | Webcam list with live previews | n/a: the App capture has no camera source (the web Camera tab uses the browser) | L | App capture has no camera source; needs Media Foundation camera capture + encoder path |
| SP-5 | Browser tab (`tab.browser`, `browserHint`) | getDisplayMedia | n/a | – | browser only |
| SP-6 | System chooser tab (`tab.picker`, `system`, `systemAction`) | When App offers only a system picker (macOS/Wayland) | n/a | – | Windows lists sources directly |
| SP-7 | Lazy previews (`CaptureSourceCard`) | Preview requested when card scrolls into view | done | – | sequential, cached across opens |
| SP-8 | Source card: title, preview, kind icon fallback | Card grid ≥240 px columns | done | – | |
| SP-9 | Refresh sources (`host.sourcePicker.refresh`, `loading`) | Spinning refresh; status "Finding sharing sources" | done (batch 6): spinning refresh | S | |
| SP-10 | Close / cancel (`common.cancel`) | ✕ button and Escape | done (batch 3): Esc | S | |
| SP-11 | Show capture border switch (`showCaptureBorder(+Hint)`) | Only when App reports `captureBorderControl` | done (batch 8): hint on hover | S | |
| SP-12 | Share sound switch (`windowAudio` / `systemAudio`, `audioOn/Off`, `audioHint`) | Disabled when the App lacks process/system audio; tooltip | done (batch 10): action + audio hint tooltip | S | |
| SP-13 | Exclude application audio (`excludeAudio`, `excludeAudioChoose/Lifetime/Hint`, `excludeNone/Missing`, `refreshApplications`) | Screens tab: pick a process whose audio is excluded | done (batch 2): "Exclude application audio" in the Screens tab with sound on; one entry per process, refresh, lifetime hint | M | App `excludeAudio` start field + `processAudioExclusion` capability exist |
| SP-14 | Empty / failed / unsupported states (`empty`, `failed`, `unsupported`, `native.incompatible`) | Comic + text | done (batch 6): no-sources comic | S | |
| SP-15 | Selection disabled while busy | Cards disabled during room mutation / switch / quality change | done: a second choice while starting is ignored | S | |
| SP-16 | Tab keyboard navigation (`navigateTabs`) | Arrow keys between tabs | done: arrow keys (batch 5) | S | |
| SP-17 | Remember last audio / border / exclusion choices | Initial values follow current share | done (batch 5): sound and border choices remembered | S | |

## 6. Sharing settings (`SharingSettings.tsx`, `QualityPresets.tsx`, `NumberControl.tsx`, `ShareMicrophoneSettings.tsx`, `MicrophoneSettings.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| SS-1 | Picture / Sound tabs (`host.settings.picture/sound`) | Segmented | done | – | |
| SS-2 | Quality presets Light/HD/High FPS (`host.quality.*`, `host.quality.title`) | 720p30 / 1080p30 / 1080p60 tiles, tooltip with Mbps | done | – | no tooltip |
| SS-3 | Quality persisted per App (`/quality-preference`) | Saved and restored | done | – | `HostEnv.Quality` |
| SS-4 | Current picture limits summary (`host.settings.pictureLimits`, `pictureDetails`) | Disclosure with W×H · fps · Mbps | done | – | |
| SS-5 | Resolution presets (`host.video.resolutionPreset`, `1440p`, `2160p`) | Slider over common values with labels | done (batch 2): 480p–2160p chips, disabled above the App limits | S | |
| SS-6 | Custom resolution width/height (`host.video.custom/width/height/inputHint/resolutionInvalid/returnCommon*`) | Number fields, even sizes, pixel cap | done (batch 2): typed width × height (even, 4–limit), invalid message | M | |
| SS-7 | Resolution limits from App (`videoCaptureLimits`) | Max size/fps from App capabilities | done (batch 2): limits from App capabilities (resolution, pixels, macroblocks/s, fps, bitrate, audio) | S | |
| SS-8 | FPS limit (`host.advanced.framerate`, `quality.invalidNumber`) | Number control with common range + custom, ceiling follows resolution | done (batch 2): slider 15–60 within the per-resolution ceiling; typed 1–ceiling; Custom + Use common range | S | |
| SS-9 | Bitrate limit (`host.advanced.bitrate`) | Number control in Mbps with ceiling | done (batch 2): slider 2–12 Mbps like the page; typed 0.1–App max; Custom + Use common range | S | |
| SS-10 | Picture preference Clarity/Balanced/Fluency (`host.advanced.preference.*(Hint)`) | Chips with hints | done | – | no hints |
| SS-11 | Video codec VP8 / Auto / H264 (`host.advanced.codec.*`) | Chips; locked while live; Auto shows resolved codec | done (batch 6): "Auto (H264)" while live, codec hints | S | |
| SS-12 | Apply feedback (`host.notice.qualityApplied/qualitySet/audioSet/partialApply`, `host.fail.quality`) | Notice after apply | done (batch 5): "Share settings applied" | S | failure becomes `host.shareEnded` (wrong) |
| SS-13 | Settings disabled while starting/switching | Controls disabled | done (batch 5): changes ignored while starting/switching | S | |
| SS-14 | Include my microphone (`host.commentary.include/description`) | Switch while live, App mic mixing | done (batch 3): switch with description | S | |
| SS-15 | Commentary mic device (`host.device.*`, `MicrophoneDeviceSelect`) | Device list, refresh, missing device, permission hint | done (batch 3) | M | App `set-microphone` takes device? verify; needs WASAPI device enumeration |
| SS-16 | Commentary mic volume (`host.microphone.volume`) | 0–200 % slider | done (batch 3): 0–200 % slider (set-microphone volume), remembered | S | |
| SS-17 | Commentary warnings (`host.commentary.duplicate/unavailable/failed`, `host.audioInput.stopped`) | Warning pills | done (batch 9) | S | |
| SS-18 | Live source audio toggle for screen shares (`host.sourcePicker.systemAudio`, `host.fail.sourceAudio`) | Switch in Sound tab while live | done (batch 1) | M | `replace-share-source` with audio flag |
| SS-19 | Live audio exclusion (`LiveAudioExclusion`) | Change excluded app while live | done (batch 2): live exclusion in the Sound tab while sharing a screen with sound | M | with SP-13 |
| SS-20 | Audio bitrate presets Standard/Music/Studio (`host.advanced.audio.*`) | Chips 64/128/192 kbps | done | – | |
| SS-21 | Custom audio bitrate (`host.advanced.audioBitrate`, `host.audio.bitrateHint`) | Number input up to App ceiling | done (batch 2): typed audio bitrate 6–320 kbps; presets above the App limit disabled | S | |
| SS-22 | Connection settings disclosure (`host.settings.connection`) | Route policy group | done | – | |
| SS-23 | Auto topology tuning (`host.advanced.route.topo(+Hint)`) | Switch, locked while live | done (batch 6): hint | S | |
| SS-24 | Privacy mode (`host.advanced.route.peerOnly`, `peerOnlyRequired`) | Locked on when no SFU, explanatory note | done (batch 9) | S | |
| SS-25 | Server media (`host.advanced.route.sfuOnly(+Hint)`) | Locked when site is SFU-only | n/a | – | App server has no SFU |
| SS-26 | Confirm in App (`native.confirmInApp`) | Android permission intent | n/a | – | |

## 7. Notices & status messages (`host-page-notices.ts`, HostPage notice state)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| NO-1 | Notice area with dismiss (`notice.dismiss`) | Operation pill under toolbar, TV-target notices on chin | done (batch 6): dismiss button | M | |
| NO-2 | Start failures (`host.fail.start`, `host.capture.*`, `native.fail.*`, `host.err.codecUnsupported`) | Mapped message + failure code | done (batch 9) | M | needs App failure codes (`reportCaptureFailure`) mapped |
| NO-3 | Share ended by App/source (`host.shareEnded`, `host.stopNotice`) | Notice | done | – | |
| NO-4 | Room closed / expired (`host.roomClosed`, `host.roomInvalid`) | Notice, room cleared | done | – | |
| NO-5 | Room code replaced (`host.roomReplaced`) | Notice | done | – | idle only |
| NO-6 | Session terminated (`host.terminated.stale/session/signal`, `host.err.alreadyConnected`) | Explains another page took over | done (batch 9) | S | |
| NO-7 | Invite rotated / revoked (`host.invite.updated/revoked`) | Notice | done (batch 1): visible while sharing | S | |
| NO-8 | Admission changes (`host.policy.setOpen/setPrivateInvite/setPrivatePassword`) | Notice | done (batch 1) | S | |
| NO-9 | Access failure (`host.accessFailed`, `host.err.updateRoomStatus`) | Notice | done: host.accessFailed | S | |
| NO-10 | Name saved offline (`host.nameOffline`) | Notice | done (batch 5) | S | |
| NO-11 | Viewers reconnecting (`host.notice.reconnecting`) | Notice | done (batch 9) | S | |
| NO-12 | Pause failures (`host.pause.*`) | Notice | done (batch 5): pause/resume failure notices | S | |
| NO-13 | Server error text passthrough | Mapped keys | done (batch 9) | S | |
| NO-14 | Screen-reader live region | `role=status` mirrors notices | n/a: Gio has no screen-reader support on Windows | – | see §15 |

## 8. Help panel, troubleshooting & debug export (`Header.tsx BrowserHelpMenu`, `ShareFailureHelp.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| HP-1 | Help popover (`help.title`) | Card anchored to ? button | done (batch 1): native Help popover | S | |
| HP-2 | Piik Web version + release link | Version, link to GitHub release tag | done (batch 8): GitHub button opens the Piik releases page | S | show App version instead |
| HP-3 | Troubleshooting guide (`help.troubleshooting`, `shareHelp.guide`) | Language-specific (zh-CN / en), project-hosted docs | done: zh/en guide | S | |
| HP-4 | Start diagnostics (`debug.start`, `debug.startHint`, `debug.startConfirm`) | Confirm, reload page with browser diagnostics | n/a: the App enables diagnostics only before the room starts (launcher button) | M | native equivalent: enable App debug logging at runtime |
| HP-5 | Export diagnostics report (`debug.export(Hint)`, `debug.exportFailed`) | Downloads browser diagnostics | done (batch 2, native form): Help → Save report writes a JSON diagnostics file to Downloads (no invite grant, tokens or names) + Show in folder | M | native: export App logs (today: press D in terminal) |
| HP-6 | Share failure help dialog (`shareHelp.title`) | Opened from failed status light: comic, reason, error code | done (batch 1): failure help inside the Help panel | M | |
| HP-7 | Failure checklist (`shareHelp.source/compare/site/appConnection/mediaConnection/permission/camera/report/appDebug`) | Steps chosen by failure code | done (batch 1) | S | with HP-6 |
| HP-8 | Copy failure details (`shareHelp.copy/copied/copyFailed`) | Copies version + label + code | done (batch 1) | S | |

## 9. Deck: room code, access, name, details, topology (`RoomChip.tsx`, HostPage deck, `ConnectionDetails.tsx`, `ViewerOverview.tsx`, `PawnDetail.tsx`, `Metrics.tsx`, `RouteTree.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| DK-1 | Room code LCD (`common.roomCode`) | Green digits | done | – | |
| DK-2 | Create room before sharing (`host.createRoom(+Hint)`, `host.roomCodePending`) | Placeholder button, busy while creating | done | – | LCD "+"; no caption/tooltip |
| DK-3 | Copy room code (`common.copy/copied/copyFailed`) | Copy button copies the **code** | done: LCD click copies the code ("Copied") | S | |
| DK-4 | Preferred room code reuse | Asks for the last code again | done | – | prefs `roomCode` |
| DK-5 | Replace room code (`host.roomReplace`) | Button | done | – | |
| DK-6 | Replace confirmation (`host.roomReplaceConfirm`) | ✓ / ✕ confirm step (ends share, disconnects viewers) | done (batch 1): second click within 5 s | S | risky: kicks the TV |
| DK-7 | Admission badge (`host.policy.currentOpen/currentPassword/currentInvite`) | Shows open / code+password / invite only | done: "Room code + password" badge | S | |
| DK-8 | Copy invite link (`host.invite.copy`, `copyAddress`, `copyFailed`) | Copies link with/without grant | done | – | |
| DK-9 | Rotate invite (`host.invite.rotate(Short)`) | New grant, old link stops | done | – | |
| DK-10 | Revoke invite (`host.invite.revoke(Short)`) | Removes grant, disconnects its viewers | done | – | |
| DK-11 | Invite link field (`host.invite`, `host.invite.address`) | Read-only selectable input, full link tooltip | done (batch 9) | S | |
| DK-12 | Skip passwords switch (`host.invite.includeCredential`, `credentialHint`) | Include grant in copied link | done | – | no hint |
| DK-13 | Invite-only warning (`host.invite.credentialRequired`) | Blocks copy when private and no password and grant omitted | done (batch 7) | S | |
| DK-14 | Empty invite pills (`host.invite.emptyOpen/emptyPassword/emptyPrivate`) | Explains what still works | done (batch 5): text follows open / private / password | S | |
| DK-15 | Open / Private policy toggle (`host.policy.open/private(+Hint)`) | Segmented with hints | done | – | no hints |
| DK-16 | Room password set/change (`host.password.setAction/changeAction/placeholder/inputPlaceholder/rule`) | Private rooms: password field | done (batch 1): private rooms: set/change password | M | API `set-viewer-password` |
| DK-17 | Show/hide room password (`host.password.show/hide`) | Eye toggle | done (batch 6) | S | with DK-16 |
| DK-18 | Remove room password (`host.password.remove`, `removed`) | Danger ✕ | done (batch 1) | S | with DK-16 |
| DK-19 | Password state marker / notices (`host.password.set/unset/saved/updated`) | Dot on key button, notices | done: password set / not set marker | S | with DK-16 |
| DK-20 | Access row collapse | Always shown with room | by design: collapsible badge | – | |
| DK-21 | Display name tag (`NameTag`) | Pill with identity colour dot | done | – | |
| DK-22 | Edit name (`host.nameEdit/name/nameSave/nameCancel`) | Inline editor, Enter saves | done | – | |
| DK-23 | Name validation error (`host.nameError`) | Red pill on invalid / >24 chars | done (batch 5): message next to the editor | S | |
| DK-24 | Name persisted (`piik:display-name`) | Restored next time | done | – | |
| DK-25 | Connection details toggle (`host.details(.show/.hide)`) | Disabled until there is data | done | – | |
| DK-26 | Local capture row (`stats.localCapture`, `host.capture.*`) | **Actual** capture resolution, fps, codec, source audio | done (batch 1): actual capture stats from the App | M | needs App `capture-state` stats |
| DK-27 | Voice rows in details (`voice.details.*`, `stats.voiceSend/Receive`) | Own voice connections, per peer send/receive | done (batch 6): room-voice links with state, RTT, loss, jitter | M | voice.go has stats via pion |
| DK-28 | All viewer connections table (`host.viewerOverview`, `ViewerOverview`) | Per viewer: route P2P/SFU, res, fps, bitrate, loss, RTT + voice columns | done (batch 2): rows clickable, route P2P like the page (relays shown in topology) | S–M | |
| DK-29 | Viewer state labels (`state.peer.connecting/routing/connected/stuttering/disconnected`) | Per-viewer status | done (batch 2): connecting / routing / connected / recent stutter | S | |
| DK-30 | Select a viewer → Pawn detail card (`PawnDetail`, `common.close`) | Per-viewer metrics + voice details, close | done (batch 2): click a viewer → detail card with close | M | |
| DK-31 | Detailed metrics (`stats.more/less`, `stats.group.*`, quality state, encoder, jitter, freezes, buffers) | Expandable metric groups | done (batch 2): Detailed metrics grouped (network, playback, source audio) from viewer evidence | M | sender stats from App `edge-quality` |
| DK-32 | Metric hints / codec warnings (`stats.*Hint`, `stats.warn.codec/audioCodec`) | Tooltips and warnings | done (batch 9) | S | |
| DK-33 | Host pawn click toggles details | Selecting host pawn opens details | done (batch 6): own pawn → menu with Connection details | S | |
| DK-34 | Topology toggle (`host.topology(.show/.hide)`) | Button | done | – | |
| DK-35 | Route tree (`RouteTree`) | Host → viewers, relay chains, P2P/SFU labels | done (batch 9) | S | |
| DK-36 | Voice links in topology | Voice routes drawn | done (batch 10): dashed voice routes with mic marks | M | |
| DK-37 | Idle / pending topology text (`host.topology.idle/pending`) | "In the room. No active media routes." | done (batch 5): idle text | S | |
| DK-38 | Select peer in topology | Opens its detail | done (batch 9) | S | with DK-30 |

## 10. Interactions: couch, chat, reactions, overlay, settings, export, person menu (`RoomInteractions.tsx`, `Couch.tsx`, `RoomReaction*.tsx`, `RoomChat*.tsx`, `FloatingPanel.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| IN-1 | Couch with pawns (host crown, "you" marker, name tags) | Seated participants | done | – | |
| IN-2 | Viewer count + Interact button (`interaction.open`) | Footer pill | done | – | |
| IN-3 | Unread badge (`interaction.unread`) | Count on Interact button | done | – | |
| IN-4 | Pawn status (connection, picture ready, stutter) | Status badge per pawn | done (batch 9) | S | |
| IN-5 | Pawn voice status (mic on/speaking/listening paused, `voice.listeningPaused`) | Badges on pawns | done (batch 6): microphone badge (green while speaking) | S | data in voiceSnapshot |
| IN-6 | Remote-control participant badge (`remote.participant.control`) | Collaboration marker | done (batch 6): remote-control badge | S | |
| IN-7 | Many viewers layout | Couch grows/wraps | done (batch 10): couch rows like Couch.tsx | S | |
| IN-8 | Click pawn → person menu (`interaction.withPerson`, `toPerson`, `express`) | Popover with target + reaction grid + Details | done (batch 3): click a pawn (or a chat avatar) → person menu | M | |
| IN-9 | Targeted reactions & throws (tomato/poop, `interaction.reaction.*`) | Throws only to another person | done (batch 3): reactions to that person; tomato/poop only with a target | S | bug: throws without target |
| IN-10 | Reaction effects over pawns (`RoomReaction`) | Bubble above sender/target, throw flight animation | done (batch 9) | M | |
| IN-11 | Gift does not override own expression (`ReactionEffects` cover rule) | Latest own reaction wins, gifts kept | done (batch 10): own expression kept, gifts beside it | S | |
| IN-12 | Interactions panel (`interaction.title`) | Floating panel bottom-right | done | – | fixed position |
| IN-13 | Move / resize panel (`floating.move/resize(+Hint)`) | Drag title/corner, arrow keys | done (batch 7): move by the title bar, resize from the lower-right corner | M | |
| IN-14 | Detach to always-on-top window (`floating.detach/focus/detachFailed`, `interaction.focus`) | Separate PiP-like window | done (batch 8): Interactions opens in its own always-on-top window (beside VibePiik); closing it brings the panel back; the Interact button raises it | M | second Gio window with TOPMOST |
| IN-15 | Chat / Voice tabs (`interaction.chat/voice`) | Tablist | done | – | |
| IN-16 | Chat log bubbles, avatars, continuation grouping | Self right, others left | done | – | |
| IN-17 | Duplicate-name labels (`labelParticipantNames`) | "Name (2)" disambiguation | done (batch 5): labelParticipantNames ported (peer ID suffix) | S | |
| IN-18 | Follow latest / back-to-latest (`interaction.latest`, `interaction.unread`) | Stops auto-scroll when reading; jump button | done (batch 3): follows the latest unless scrolled back; "Back to latest" / "N new messages" | S | |
| IN-19 | Empty chat (`interaction.empty`) | Icon + text | done | – | |
| IN-20 | Composer (`interaction.placeholder`, `send`) | Input, Enter sends, IME-safe, busy while pending | done | – | |
| IN-21 | Character count (`MAX_CHAT_CODE_POINTS`) | "n / 280" | done | – | |
| IN-22 | Interaction errors (`interaction.error.*`, `interaction.reconnecting`) | Status line | done | – | |
| IN-23 | React button + reaction grid (`interaction.express`, `moreReactions`) | Self reactions picker | done (batch 3): composer grid untargeted (14), person menu targeted (16) | S | |
| IN-24 | Quick reactions from chat avatar (`RoomQuickReactions`) | 4 quick choices + more | done (batch 3): chat avatar opens the person menu | S | |
| IN-25 | Latest reaction activity strip (`RoomReactionActivity`) | Sender → target + effect inside panel | done (batch 6) | S | |
| IN-26 | On-screen chat toggle (`interaction.overlay.show/hide(+Hint)`) | Danmaku button in composer and preview bar | done (batch 4): toggle in the chat tools, remembered | S | needs RS-28 to be useful |
| IN-27 | Chat settings page (`interaction.settings(+Hint)`) | Sliders/back button | done (batch 4): Chat settings page in Interactions | S | |
| IN-28 | Overlay text size / opacity / speed / colour by sender (`interaction.overlay.*`) | Persisted per room prefs | done (batch 4): text size 80–130 %, opacity 40–100 %, speed 50–150 %, participant colours | S | |
| IN-29 | Export chat (`interaction.export`, `exportLimit`, `ephemeral`) | Save latest 1000 messages as .txt | done (batch 3, native form): save the kept chat as UTF-8 text in Downloads | S | Save dialog or Downloads |
| IN-30 | Screen-reader reaction log (`interaction.receivedReaction/expressedReaction`) | aria-live text | n/a: Gio has no screen-reader support on Windows | – | §15 |
| IN-31 | Details shortcut in person menu (`host.details`) | Opens viewer detail | done (batch 3): Connection details from the person menu (opens the viewer card) | S | with IN-8 |

## 11. Voice (`RoomVoicePanel.tsx`, `RoomVoiceControls`, `RoomVoiceSettings`, `MicrophoneSettings.tsx`, `VoiceConnectionDetails.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| VO-1 | Open room voice (host) (`voice.open`, `voice.openHint`) | Joins + enables voice for the room | done | – | hint only as empty-state text |
| VO-2 | Close room voice (`voice.close`, `voice.closeHint`) | Danger button, ends voice for all | done | – | |
| VO-3 | Microphone on/off (`host.device.microphone`, `voice.microphoneOn/Off`) | Toggle, busy while authorising | done | – | |
| VO-4 | Cancel mic request (`voice.captureCancel`) | Click while pending cancels | done | – | |
| VO-5 | Mic level meter on button (`MicrophoneLevel`) | Live level overlay | done (batch 3): live level meter under the microphone volume in Voice settings | S | `micLevel` already computed |
| VO-6 | Mic volume 0–200 % (`host.microphone.volume`, `AudioVolumeMenu`) | Popover slider from mic button | done (batch 3): room voice microphone volume 0–200 %, remembered | S | scale PCM before encode |
| VO-7 | Listening on/off / deafen (`voice.listening`, `voice.deafen/undeafen(+Hint)`) | Toggle | done | – | |
| VO-8 | Master listening volume 0–200 % (`voice.listeningVolume(+Hint)`) | Slider, persisted | done (batch 2): Voice settings → Master voice volume 0–200 %, saved | S | scale in `mix` |
| VO-9 | Participants list (`voice.participants`, `voice.participant`, `voice.self`) | Pawns, names, self tag, host crown | done (batch 9) | S | |
| VO-10 | Member mic state + speaking (`voice.microphoneActive/Inactive`, `voice.speaking`) | Icon, level meter, tooltip | done (batch 10): level meter + tooltip | S | |
| VO-11 | Member listening paused (`voice.listeningPaused`) | Headphones-off badge | done | – | |
| VO-12 | Mute a person for me (`voice.listenPerson`, `muteForMe`) | Speaker toggle | done | – | click on mic icon (less discoverable) |
| VO-13 | Per-person volume 0–200 % (`voice.personVolume`) | Slider + % readout | done (batch 2): click a participant → individual volume 0–200 % | S | per-playback gain |
| VO-14 | Avatar → reactions to person | Opens quick reactions | done (batch 6) | S | with IN-8 |
| VO-15 | Voice settings page (`voice.settings`) | Back button, settings scroll | done (batch 3): Voice settings: microphone, volumes, processing, bitrate | M | |
| VO-16 | Microphone device choice (`host.device.default/missing/refresh/listFailed/permission.microphone`) | Device select + refresh, persisted | done (batch 3): Windows microphones (Core Audio endpoints) with refresh; the echo canceller and the plain path both use the choice | M | WASAPI/waveIn device enumeration; `openMicrophone` uses default |
| VO-17 | Noise & echo reduction (`host.microphone.voiceProcessing(+Hint)`) | Switch, persisted | done (batch 3): Noise & echo reduction switch (Voice Capture DSP on/off), remembered | L | needs AEC/NS (e.g. WebRTC APM, RNNoise) natively |
| VO-18 | Room voice bitrate (host) (`voice.bitrate(+Hint)`, `voice.quality.saver/standard/clear`) | Presets + kbps entry, applies to everyone | done (batch 2): Saver/Standard/Clear + typed 6–320 kbps (room-voice-command bitrate) | S | command `{action:"bitrate"}` |
| VO-19 | Voice unavailable / reconnecting hints (`voice.unavailable`, `voice.reconnecting`, `voice.offline`) | Status lines | done (batch 10): reconnecting / offline / unavailable lines | S | |
| VO-20 | Voice errors (`voice.error.*`) | Alert line | done (batch 9) | S | mic open failure keys missing |
| VO-21 | Autoplay unblock (`voice.play`) | Button when browser blocks audio | n/a | – | |
| VO-22 | Voice connection details (`voice.details.title/joined/partial/unavailable/sendTo/receiveFrom/forwardTo/via/codec`) | Rows in details and pawn detail | done (batch 6) | M | see DK-27 |
| VO-23 | Voice input preferences persisted (`voiceInput`, `voiceListeningVolume`) | Restored | done (batch 3): device, volumes and processing remembered in ui.json | S | |

## 12. Remote control — host side (`HostRemoteControl.tsx`, `RemoteControlRequestCard.tsx`, `RemoteShortcutSelect.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| RC-1 | Remote control panel (`remote.title`) | Floating, anchored, detachable | done (batch 8): Remote control detachable too | S | |
| RC-2 | Pending request count badge (`remote.requests`) | Number on toolbar button | done (batch 2): red count badge on the toolbar button | S | native opens panel automatically instead |
| RC-3 | Trigger title reflects state (`remote.controlling`, `remote.hostFocus`) | Label changes | done (batch 2): toolbar hint follows requests / controlling / host focus | S | |
| RC-4 | Enable switch | On/off | done | – | |
| RC-5 | Consent gate for Room / Password modes (`remote.enable`, `remote.enableRoom`) | Enabling with room/password requires explicit confirm button | done (batch 2): Room/Password ask for confirmation (Allow Viewers / Enable, Cancel) | S | safety |
| RC-6 | Authorization modes Ask / Room Viewers / Password (`remote.mode.*`) | Segmented | done | – | |
| RC-7 | Mode help text (`remote.help.ask/room/password`) | Note under modes + tooltips | done (batch 2) | S | |
| RC-8 | Control password + Apply (`remote.password`, `remote.apply`) | Password field, submit | done | – | |
| RC-9 | Remembered mode (`remoteControlMode` pref) | Draft restored next time | done (batch 2): remembered in ui.json (remoteControlMode) | S | |
| RC-10 | Enabling pending + cancel (`remote.cancel`) | Spinner and Cancel | done (batch 6) | S | |
| RC-11 | Simultaneous control (`remote.simultaneous`, `simultaneousHelp`) | Switch + note | done (batch 2): help note under the switch | S | |
| RC-12 | Gamepad note + setup guide (`remote.gamepadHostHelp`, `remote.gamepadSetup`) | Link to docs (zh/en anchor) | done (batch 2): gamepad note + "Gamepad setup" opens the guide (en/zh) | S | relevant for TV gaming |
| RC-13 | Requests list (`remote.waiting`, `remote.controlling`, `remote.hostFocus`) | Name, state, pending last | done | – | |
| RC-14 | Allow / Decline / End (`remote.allow/decline/stop`) | Buttons per request | done | – | ✕ for active |
| RC-15 | Consent note (`remote.consent`) | Shown with pending requests | done (batch 2): shown while a request is pending | S | |
| RC-16 | Per-action failure (`remote.decisionFailed`, `remote.macPermission`) | Inline alert | done (batch 2): failure shown inside the panel | S | |
| RC-17 | Policy lifetime note (`remote.policyLifetime`) | Note | done | – | |
| RC-18 | Host stop shortcut (`remote.shortcut`, `remote.hostStopKey(+Help)`) | Select Ctrl+Alt+F1…F12 | done (batch 2): stepper F1–F11 with "Host stop shortcut" help | S | |
| RC-19 | Stop shortcut persisted (`remoteStopKey`) | Saved per site, restored on enable | done (batch 2): saved in ui.json (remoteStopKey) and applied when sharing starts | S | |
| RC-20 | Shortcut failure (`remote.shortcutFailed`) | Alert, enabling blocked | done | – | |
| RC-21 | Co-play presence | Approved controllers sent to room | done | – | `flushCoPlay` |
| RC-22 | Disabled while paused/switching | Controls disabled | done (batch 2): controls disabled while paused/switching | S | |
| RC-23 | Linux/macOS notes (`remote.linuxStop/linuxEnable`) | Platform notes | n/a | – | |

## 13. Tooltips, comics, visual ✦ mode (`Tooltip.tsx`, `Comic.tsx`, `hints/*.tsx`, `comic-presentation.ts`, `BrandMark.tsx`)

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| TT-1 | Hover/focus tooltips with text | Nearly every control has a tooltip (title + hint text) | done (batch 1): hover tooltips after 500 ms | M | generic Gio tooltip widget (hover delay, placement, keyboard focus) |
| TT-2 | Hint comics in tooltips (87 `hint-*` scenes, sets 1–4 + playback/controls/metrics/admission/interactions/remote/relay/voice) | Animated 320×96 scenes per control | done as stills (batch 5): 179 web comics captured from the tooltip preview page (reduced-motion pose, dark/light) and shown in tooltips of the main controls | L | port SVG scenes via `svg.go`; animation timelines |
| TT-3 | State comics (77 kinds: signal, starting, paused, failed, no-audio…) | In pills, LED tooltip, overlays, failure dialog | done as stills (batch 5): starting / switching / paused / share ended / failed / room invalid on the TV | L | |
| TT-4 | Comic tone/motion (`still`, `progress`, `demo`) | Tinted, animated by state | done (batch 7): comics animate (16 frames over the 3.2 s loop, captured from the web page): tooltips play once when shown, progress comics on the TV loop | M | |
| TT-5 | Overflow tooltip for truncated names | Full name on hover | done (batch 10): pawn tooltip with the full name; tags ellipsize in their seat | S | |
| TT-6 | Brand mark motion (`motion="once"`, loader) | Animated mascot | done (batch 10): launcher mascot draws in and winks, replays on hover | S | |
| TT-7 | Animated glyph draws (`draw=` on buttons) | Icon draw-in animations | done (batch 10): TV action icons draw in (wipe) | M | cosmetic |
| TT-8 | ✦ visual mode: captions hidden | Text hidden, icons/comics carry meaning | done (batch 5): toolbar and tab captions hidden in ✦ mode, caption becomes the hint | M | |
| TT-9 | ✦ mode: comics replace text | Empty states, notices as comics | done (batch 7): ✦ tooltips show the animated comic alone | L | depends on TT-2/3 |
| TT-10 | ✦ mode keeps a spoken language for a11y | vis + zh/en | done (batch 10): ✦ keeps the spoken language for tooltips | S | |
| TT-11 | Reduced motion (`prefers-reduced-motion`) | Animations reduced | done (batch 6): follows Windows "Animation effects" | S | Windows "Show animations" setting |

## 14. Chinese / i18n

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| I18-1 | Full zh catalogue for all copy keys | `zh.ts` | done | – | `copy_gen.go` generated |
| I18-2 | CJK font shaping | Browser fonts | done | – | `newShaper(zh)` |
| I18-3 | Default language from system | Browser language | done | – | Windows UI language |
| I18-4 | PIIK TV copy in zh | `piiktv-copy` | done | – | `piiktv_copy.go` |
| I18-5 | Welcome lines zh/en | playful.welcome | done | – | |
| I18-6 | Title frames / playful waiting lines | Rotating zh/en/visual variations | done (batch 10): window title frames + rotating waiting lines (en/zh/it) | S | with HD-14 |
| I18-7 | Language-specific doc links (`troubleshooting.zh-CN.md`, `remote-control.zh-CN.md#手柄`) | zh users get zh docs | done (batch 5): Chinese troubleshooting guide | S | |
| I18-8 | Language/theme handed to web pages | Launch URL carries `lang/vis/theme` | done (batch 6): join page gets language, ✦ mode and theme | S | use `clientLaunchURL` equivalent |
| I18-9 | Theme captions via copy keys (`theme.dark.short`) | Copy keys | done (batch 5) | S | |
| I18-10 | Copy fallback for unknown keys | – | by design: empty value is intentional | S | add a test that every `u.T` key exists |

## 15. Keyboard & accessibility

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| KB-1 | Tab focus order across controls | Native browser order + focus rings | done (batch 9) | M | |
| KB-2 | Enter submits forms (name, chat, password, TV link) | Form submit | done | – | |
| KB-3 | Escape closes picker / popovers / dialogs | Yes | done (batch 3): Esc closes picker, person menu, help, remote, reactions, Interactions, viewer card, settings (topmost first) | S | |
| KB-4 | Arrow keys in segmented controls / radio tiles | `navigateTabs`, launcher radios | done (batch 5): arrow keys move the selection in segmented controls (wrap-around) | S | |
| KB-5 | Floating panel keyboard move/resize | Arrow keys | done (batch 10): arrow keys on the focused title bar / resize grip | S | with IN-13 |
| KB-6 | Space toggles preview playback | Yes | done (batch 6): Space | S | |
| KB-7 | Screen reader names, roles, live regions | aria-* everywhere | n/a: Gio has no screen-reader support on Windows | L | Gio has no UI Automation provider on Windows |
| KB-8 | Focus restore after actions (site removal, menus) | Yes | done (batch 10): focus moves to the next site after removal | S | |
| KB-9 | Autofocus (picker close, name editor, password fields) | Yes | done (batch 9) | S | |

## 16. Viewer page / Join a room (`ViewerPage.tsx`, `JoinPage.tsx`, `RemoteControl.tsx`, `RemoteKeyboard.tsx`, `RemoteMouse.tsx`) — opened today in the WebView window

| ID | Feature | Web behaviour | Native status | Effort | Notes |
|---|---|---|---|---|---|
| VI-1 | Site access gate (`gate.*`) | Passphrase form, cookie | web window | M | |
| VI-2 | Join page (`join.title`, `join.hint`, door comic) | 4-digit code entry | web window | M | |
| VI-3 | Invite link entry (`/r/<code>#v=<grant>`) | Joins with grant | web window | S | |
| VI-4 | Room code input + error shake (`RoomCodeInput`, `gate.malformed(Hint)`) | Validation 1000–9999 | web window | S | shared with RS-6 |
| VI-5 | Access denied / not found / closed / full / invalid invite (`viewer.msg.*`, `viewer.hint.*`) | Comic + message + hint | web window | S | |
| VI-6 | Room password retry (`join.passwordAction`, `join.password`, `join.passwordRule/Error`) | Expandable password form | web window | S | |
| VI-7 | Join progress (`viewer.msg.joining/waitingHost/preparingP2p/allocating/receiving/connectionRetry`) | Overlay with comic + retry counter | web window | M | |
| VI-8 | Video playback of host picture | `<video>` with H.264/VP8 + Opus | web window | L | native video decode + render |
| VI-9 | Tap to play / autoplay (`viewer.msg.needsPlay`) | Click overlay | web window | S | |
| VI-10 | Host paused / host offline / recovering overlays & notices (`viewer.msg.hostPaused`, `viewer.notice.*`) | Overlays + pills | web window | S | |
| VI-11 | Playback controls: play/pause, mute, volume 0–200 % (`playback.*`) | Bar | web window | M | |
| VI-12 | Fit / rotate / theater / fullscreen / PiP | Bar | web window | M | |
| VI-13 | Playback settings: on-screen chat, route Auto/P2P/SFU (`playback.route.*`) | Popover | web window | M | |
| VI-14 | Relay volunteer + capacity (`playback.relayVolunteer`, `relayLimit`) | Viewer forwards to other viewers | web window | L | RTP forwarding (pion can) |
| VI-15 | Reconnect (`viewer.reconnect`) | Button | web window | S | |
| VI-16 | Interactions (chat, reactions, voice join/leave `voice.join/leave`, `voice.closedHint`) | Same panel as host | web window | M | reuse hostSession interaction + voiceEndpoint |
| VI-17 | Deck: room code, host name, own name edit | Summary row | web window | S | |
| VI-18 | Details: shared media, relay data, downstream (`stats.sharedMedia/relay/downstream`), voice | Metrics rows, pawn detail for relay children | web window | M | |
| VI-19 | Topology (viewer view, `selfPeerId`) | Route tree | web window | S | |
| VI-20 | Request / cancel / end control (`remote.request/cancel/stop/pending/waiting/failed/busy`) | Buttons on bar | web window | M | |
| VI-21 | Remote mouse: direct touch / trackpad, virtual mouse expand/collapse, wheel (`remote.mouse*`, `touchMode.*`) | Pointer mapping to picture | web window | L | |
| VI-22 | Game mouse pointer lock (`remote.gameMouse*`, `releaseMouse`, `lockFailed`) | Relative mouse, Esc to release | web window | M | Win32 raw input / ClipCursor |
| VI-23 | Remote keyboard: pages, combination mode, hold-to-press, special keys (`remote.keys.*`, `remote.key.*`) | On-screen keyboard + physical keys | web window | L | low-level keyboard hook to capture Win/Alt+Tab |
| VI-24 | Type remote text ≤2 KB (`remote.text`, `remote.textTooLong`) | Text box | web window | S | |
| VI-25 | Gamepad forwarding (`remote.gamepadHelp`, `remote.reason.gamepad`) | First standard controller | web window | M | XInput polling |
| VI-26 | Exit-control shortcut (`remote.releaseKey(+Help)`) | Saved per browser | web window | S | |
| VI-27 | Pinch zoom / reset zoom (`remote.touchHelp`, `remote.resetZoom`) | Touch gestures | web window | M | |
| VI-28 | Document title (watching/waiting frames) | Rotating title | web window | S | |

---

## Priorities for a host streaming games to a TV

1. **Shared sound volume / mute for all viewers** (RS-18, RS-19). The game is often too loud or too quiet on the TV, and the host has no control. The App command already exists.
2. **Visible notices while sharing, plus cancel-start and switching state** (NO-1, NO-2, RS-9, RS-14, SC-2, SS-12). Today, failures during a live share are silent, a quality failure is reported as "share ended", and a hung start cannot be cancelled.
3. **Confirm before Replace room code** (DK-6). One misclick disconnects the TV.
4. **Live preview of screen shares** (RS-11). Fullscreen games are usually shared as a display, and the TV-side picture can't be verified without it.
5. **Exclude application audio + live source-audio toggle** (SP-13, SS-18, SS-19). Keeps Discord/voice chat or notification sounds off the TV when sharing a screen. The App capability exists.
6. **Actual capture stats and per-viewer detail** (DK-26, DK-28, DK-30, DK-31). Needed to tell whether the TV link is bandwidth-limited or encoder-limited. Also fix the blank `stats.picture` header (I18-10).
7. **Picture settings fidelity** (SS-6, SS-7, SS-8, SS-21). Use App limits, per-resolution FPS ceilings and typed values for 1440p/4K TVs.
8. **Remote control for couch gaming** (RC-12, RC-2, RC-5, RC-19, RC-7). Gamepad setup link, request badge, the consent gate, and persisted stop shortcut and mode.
9. **Room password management** (DK-16..19, DK-13). Lets the host run a private room that the TV joins by code.
10. **Share-failure help and diagnostics export** (HP-6..8, HP-5). Needed to support users when a share fails.
11. **Tooltips** (TT-1). Cheap overall, and they unlock most hint texts. Comics (TT-2/3) and ✦ mode (TT-8/9) can come later.
12. **Window title with room code/status** (HD-14), **launcher load-failure retry** (LA-4), **wire inert buttons** (HD-6, LA-9/11/12/13/28).

## What a native viewer / Join would require

A native viewer needs:
- **Viewer signaling** in `hostSession` style: authenticate as `viewer` with an invite grant or code+password, site-passphrase cookie admission, route assignment, restart requests, viewer-quality-evidence reports and route preferences.
- **pion receive path** for the host's H.264/VP8 video and Opus audio. Relay volunteers also have to forward RTP to their child viewers, the same way `voice.go` already forwards voice.
- **Video decoding**: Media Foundation/D3D11 hardware H.264 and libvpx/openh264 fallback, with a jitter buffer, A/V sync and freeze handling.
- **Rendering** decoded frames into Gio as GPU textures, so fit, rotation, the danmaku overlay, fullscreen and PiP can be drawn. DWM thumbnails cannot be used here.
- **Audio playout** with 0–200 % gain, through the existing speaker path.
- **Viewer UI**: the join/gate/password screens, the progress and failure overlays, and the playback bar.
- **Remote-control client**: data channel, a low-level keyboard hook, raw-input relative mouse/pointer lock, XInput gamepad, on-screen keyboard and text typing.
- **Reuse** of the existing interactions, voice, couch and deck code.

This is an L-sized project (several weeks). The decoder/renderer is the critical path. Until then, opening the viewer in the WebView is the pragmatic choice. It should at least receive the native language/theme (I18-8). "Join a room" should also stop silently ending the native host session (RS-5).

## Native-only additions

- Chat over other apps (batch 4): chat and reactions in a corner of a chosen screen, above games. Separate topmost layered window, click-through, never activated, excluded from capture (WDA_EXCLUDEFROMCAPTURE); no injection or hooks, so anti-cheat is not involved. Needs borderless/windowed full screen. Settings: screen, corner, time on screen, test message.
- Italian (batch 6): selectable language (中 / EN / IT / ✦); Windows language first, English for any other. Web catalog translated in copy_it.go; PIIK TV and native texts in lang_it.go.
