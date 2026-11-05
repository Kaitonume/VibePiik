package nativeui

import "image/color"

// Palette is the web client's design tokens (src/client/ui/palette.css).
type Palette struct {
	Dark bool

	Wall, Wall2, Paper, Ink, Muted, Line, ControlEdge              color.NRGBA
	Primary, PrimaryInk, PrimaryHover                              color.NRGBA
	Action, ActionDeep, ActionInk, ActionText                      color.NRGBA
	ControlHover, ControlSelected, ControlSelectedInk              color.NRGBA
	Idle, Busy, Live, Warn, Danger, DangerText, DangerHover        color.NRGBA
	SurfaceRim, Shade                                              color.NRGBA
	GlazeTop, GlazeBottom                                          color.NRGBA
	Dots, SegmentText                                              color.NRGBA
	LCD, LCDDigits, Stage, Frame, FrameHighlight, Couch, CouchDark color.NRGBA
	BrandDot, WelcomeStar, Sparkle                                 color.NRGBA
	NoticeWarn, NoticeDanger                                       color.NRGBA
	ShadeAlpha                                                     float32
}

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

func rgba(r, g, b uint8, a float32) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: uint8(a*255 + 0.5)}
}

// withAlpha scales the color's alpha by a.
func withAlpha(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A)*a + 0.5)
	return c
}

func LightPalette() Palette {
	return Palette{
		Wall: rgb(0xf3f5f8), Wall2: rgb(0xe5eaf2), Paper: rgb(0xfffefb), Ink: rgb(0x29374a),
		Muted: rgb(0x5f6976), Line: rgb(0xd4d6d8), ControlEdge: rgb(0x858d96),
		Primary: rgb(0x2765ce), PrimaryInk: rgb(0xffffff), PrimaryHover: rgb(0x2259b5),
		Action: rgb(0x3478e5), ActionDeep: rgb(0x2765ce), ActionInk: rgb(0xffffff), ActionText: rgb(0x2055b2),
		ControlHover: rgb(0xeef3ff), ControlSelected: rgb(0xe5efff), ControlSelectedInk: rgb(0x214c9b),
		Idle: rgb(0x8ea3b8), Busy: rgb(0x3478e5), Live: rgb(0x2fa66a), Warn: rgb(0xd98e04), Danger: rgb(0xd64541),
		DangerText: rgb(0xa24144), DangerHover: rgb(0xfbece8),
		SurfaceRim: rgba(255, 255, 255, .95), Shade: rgba(36, 53, 78, .18), ShadeAlpha: .18,
		GlazeTop: rgba(255, 255, 255, .6), GlazeBottom: rgba(51, 89, 147, .025),
		Dots: rgb(0x78828f), SegmentText: rgb(0x6d7783),
		LCD: rgb(0x1d2b22), LCDDigits: rgb(0x9de8bf), Stage: rgb(0x17191c), Frame: rgb(0x283445), FrameHighlight: rgb(0x3e4b5b),
		Couch: rgb(0xe9ad63), CouchDark: rgb(0xcd8845),
		BrandDot: rgb(0xe77d38), WelcomeStar: rgb(0xd7982b), Sparkle: rgb(0xd08a22),
		NoticeWarn: rgb(0xfdf7ec), NoticeDanger: rgb(0xfdf3f0),
	}
}

func DarkPalette() Palette {
	return Palette{
		Dark: true,
		Wall: rgb(0x242a34), Wall2: rgb(0x2c3543), Paper: rgb(0x354153), Ink: rgb(0xf6f3eb),
		Muted: rgb(0xc6c7c5), Line: rgb(0x5c6571), ControlEdge: rgb(0x92969c),
		Primary: rgb(0x2765ce), PrimaryInk: rgb(0xffffff), PrimaryHover: rgb(0x2259b5),
		Action: rgb(0xf6f3eb), ActionDeep: rgb(0xf6f3eb), ActionInk: rgb(0x29374a), ActionText: rgb(0xf6f3eb),
		ControlHover: rgb(0x414f67), ControlSelected: rgb(0xf6f3eb), ControlSelectedInk: rgb(0x29374a),
		Idle: rgb(0x8ea3b8), Busy: rgb(0x82b1ff), Live: rgb(0x3fb97a), Warn: rgb(0xe8a33d), Danger: rgb(0xe25a52),
		DangerText: rgb(0xe88880), DangerHover: rgb(0x464453),
		SurfaceRim: rgba(222, 234, 255, .12), Shade: rgba(6, 12, 22, .38), ShadeAlpha: .38,
		GlazeTop: rgba(224, 237, 255, .06), GlazeBottom: rgba(0, 0, 0, .035),
		Dots: rgb(0xa1a3a4), SegmentText: rgb(0xb8baba),
		LCD: rgb(0x0d1611), LCDDigits: rgb(0x9de8bf), Stage: rgb(0x141517), Frame: rgb(0x1d2633), FrameHighlight: rgb(0x3e4b5b),
		Couch: rgb(0xe9ad63), CouchDark: rgb(0xcd8845),
		BrandDot: rgb(0xe77d38), WelcomeStar: rgb(0xd7982b), Sparkle: rgb(0xf4c14d),
		NoticeWarn: rgb(0x404752), NoticeDanger: rgb(0x3f4353),
	}
}
