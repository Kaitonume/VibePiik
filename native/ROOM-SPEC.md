# Host room — measured from the web host page (dark, 1200×880, idle, App connected)

Viewport content width 1185 (scrollbar). Page scrolls (total height 1057).

## Header (lr-top, h64, padding 14 44 6)
- brand mark 34 at (50,19) inside a 46×44 link.
- right group (gap 10): LEDs pill `lr-leds` 192×33 (bg wall-2 #2c3543, r999, pad 8 12, 3 dots 6px gap 4 color mix
  dots, label 11.5/700 @.8) → "Not connected to server" / "Server connected" (dots live green);
  TV send button (compact h44, disabled until room) ; App button (compact, glyph check, "App") ; lang segmented 108×44 ;
  theme toggle ; help quiet 44×44.

## Scene (lr-scene 1100 wide, centred, top 72)
- TV `lr-tv` 960×567: bg linear-gradient(155deg,#3e4b5b,#1d2633 75%), border 3 #1d2633, radius 26 26 20 20,
  shadow inset 0 1 rgba(255,255,255,.16), 0 2 0 shade(.38), 0 18 35 shade(.22); padding 12 12 0.
  - screen 930×523 (16:9) bg #141517 (light #17191c), radius 14 14 8 8.
  - welcome line top 32 into screen: "✦ " (#d7982b) + text 16/600 lh 27.2 #dfe8f2, centred.
  - entry actions centred (gap 18): round 84×84 buttons, border 3, glyph 34:
    Start sharing = bg #2765ce border #2765ce white "cast" glyph; Join a room = bg rgba(255,255,255,.07) border #dfe8f2.
    caption under (8px) 14.5/600 #dfe8f2 text-shadow 0 1 8 rgba(0,0,0,.65).
  - chin h26: status indicator 24×24 centred, moon glyph (idle #8ea3b8) 17px.
- host controls row (top +12 after TV, h58): centred toolbar with "Sharing settings" lr-btn (h44, glyph sliders).
- couch `lr-couch` 720 wide, h140 (top +20): benches svg 280×108 viewBox 0 0 280 108:
  legs `M38 98v7m204-7v7` stroke #1d2633 w6 round; rect 18,32 244×65 rx24 couch-dark #cd8845; rect 28,24 224×60 rx23 couch
  #e9ad63; rect 28,76 224×24 rx10 couch; seams `M93.33 79v16m93.33-16v16` couch-dark w2; arms rect 14,59 25×39 rx12 and
  241,59 25×39 rx12 couch-dark.
  - pawn 48 wide at seat centre (top +24 in couch): person svg 40×48 viewBox 48×56: head circle 24,14 r8, body
    `M10 47c0-13 5-20 14-20s14 7 14 20q0 3-3 3H13q-3 0-3-3Z`, eyes ellipses (21.2,14.1)/(26.8,14.1) rx1.1 ry1.5 #263b43,
    host crown (translate 18,0) `m1.2 6.3-1.2-3.7q-.2-.7.5-.4L3 3.6l2.4-2.5q.6-.7 1.2 0L9 3.6l2.5-1.4q.7-.3.5.4l-1.2 3.7q-.2.9-1.2.9H2.4q-1 0-1.2-.9Z`
    fill #f7d861 stroke #705025 w1 + `M2.1 6.5h7.8` #b58a2f. Host colour rgb(164,116,213).
  - "you" marker: triangle 13×10 #2fa66a, 13px above the pawn, centred.
  - name tag under the pawn: 11px weight 850, paper bg (#354153 dark), radius 7, pad 1 6, margin-top -4.
  - footer: count pill (people glyph 14 + number) mono 11, bg paper, border 1 wall-2, r999, pad 4 8.
## Deck (lr-deck, top +34, row 1100×90): bg paper, border 1 control-edge, radius 24, pad 15 18, gap 12 8.
- "Room code" field cap 12/600 ink; LCD 173×58: empty = bg wall-2, border 3 dashed control-edge, r12, pad 8 12,
  4 "-" digits mono 24/700 letter-spacing 7.2 muted + "+" action 34; filled = bg #0d1611 (dark) border 3 ink, digits #9de8bf + copy glyph.
- name tag 132×40: bg paper, border 2 ink, r999, 13.5/700, dot 9 host colour, "Host"; Edit button (h52).
- Connection details (disabled until media) / Connection topology buttons (h52).
