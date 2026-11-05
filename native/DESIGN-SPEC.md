# VibePiik native UI — visual spec (extracted from the web client)

Source of truth: `src/client/styles.css` (S), `src/client/ui/palette.css`, component CSS. Gio port must match.

## Theme
- `localStorage["piik:ui-theme"]` → native: own setting; default follows Windows (AppsUseLightTheme registry).
- Palette Light / Dark:
  wall #f3f5f8/#242a34; wall-2 #e5eaf2/#2c3543; paper #fffefb/#354153; ink #29374a/#f6f3eb;
  muted #5f6976/#c6c7c5; line #d4d6d8/#5c6571; control-edge #858d96/#92969c;
  primary #2765ce (both), primary-ink #fff; action #3478e5/#f6f3eb; action-deep #2765ce/#f6f3eb;
  action-ink #fff/#29374a; action-text #2055b2/#f6f3eb; control-hover #eef3ff/#414f67;
  control-selected #e5efff/#f6f3eb; control-selected-ink #214c9b/#29374a; idle #8ea3b8;
  busy #3478e5/#82b1ff; live #2fa66a/#3fb97a; warn #d98e04/#e8a33d; danger #d64541/#e25a52;
  surface-rim rgba(255,255,255,.95)/rgba(222,234,255,.12); shade rgb(36,53,78)@.18 / rgb(6,12,22)@.38.
- glaze: light linear-gradient(165deg, rgba(255,255,255,.6), transparent 65%, rgba(51,89,147,.025)); dark (224,237,255,.06)…(0,0,0,.035).
- Fixed: couch #e9ad63 / #cd8845; you #2fa66a; LCD #1d2b22 (dark #0d1611) digits #9de8bf; stage #17191c/#141517;
  TV frame #283445/#1d2633 highlight #3e4b5b; brand dot #e77d38; welcome star #d7982b.
- Mixes: dots rgb(120,130,145)/rgb(161,163,164); segment text rgb(109,119,131)/rgb(184,186,186);
  primary hover #2259b5; danger text #a24144/#e88880; danger hover bg #fbece8/#464453;
  notice warn #fdf7ec/#404752, danger #fdf3f0/#3f4353.

## Type (Segoe UI; mono Cascadia Mono → Consolas)
body 15/1.5; wordmark h1 36 bold, lh 1.1, -0.04em, "App" 15/650 muted +6px; button caption 12.5/600;
section label 13/700 muted; welcome 15/600 lh1.7 centred; mode tile title 14 bold / hint 11 @.7;
standby strong 14, p 12 lh1.65 muted; access hint 12 lh1.6; footer 11.5 muted; LED label 11.5/700 @.8;
TV msg 14.5/600 #dfe8f2 shadow; LCD mono 24/700 .3em; name tag 13.5/700; pill 13/600; tooltip 600 14/1.6;
pawn name 11/700; quality caption mono 11; picker title 15; tabs 12/700.

## Shape
radius 16, control radius 18; pills/buttons 999; quiet btn 12; tiles/inputs 18; cards 22 (standby, PiikTV), 28 approval,
24 sharing panel/deck row, 20 anchored, 14 tooltip, 12 LCD, 8 preview. TV frame 26/26/20/20, screen 14/14/8/8.
Borders: controls 2px control-edge; cards 1px line/control-edge; LCD/name tag 2.5–3px ink; TV 3px.
control-shadow: 0 3px 0 shade(.18/.38), 0 5px 10px shade(.081/.171). panel-shadow: inset 0 1px 0 rim, 0 3px 0 shade, 0 16px 36px shade×.55.
Focus 3px action offset 2 radius 8. Control heights 52/44/36, icon 18 (16 dense), padding 14/10/8.

## Layout @1200×880
Header padding 14 44 6 gap 12. Launcher: centred grid padding 24 16 40; panel ≤620 column gap 20; heading ≤380;
room-settings ≤460 auto-fit min 210 gap 12. Room: ≤1180 padding 8 40 64; TV ≤960 padding 12 12 0, 16:9 screen, chin 26;
couch ≤720 mt 20; deck mt 34 gap 14; sharing settings ≤820.

## Components (see full notes in session report)
Button (lift -2 hover, +2 press, spring cubic-bezier(.3,1.45,.45,1) 260ms), radio cards (min-h 128, pad 18, glyph 27,
selected = control-selected + action border + 1px inset ring), inputs (h48 r18), disclosure, standby card, PiikTV card,
LCD room code, header (lang segmented 中/EN/✦, theme toggle), LED strip, tooltips (500ms delay), switch 58×34,
segmented toggle, source picker (grid minmax 240, preview 144), sharing settings, host deck, share toolbar, TV, couch, pawns.

## Assets
BrandMark viewBox 32 (BrandMark.tsx:39-90); Glyphs 24×24 stroke 2 (ui/icons.tsx); comics 320×96 (hints/*.tsx).
