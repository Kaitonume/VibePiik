package nativeui

import (
	"image"
	"image/color"
	"sync"

	"gioui.org/f32"
	"gioui.org/layout"
)

// Reaction art: small vector drawings in a 24×24 box (the web client uses
// colour emoji, which Gio cannot draw; tomato and poop are RoomReaction.tsx's
// own 40×40 art scaled down).

type artPart struct {
	svg    string
	color  color.NRGBA
	stroke float32 // 0 = filled
}

var (
	faceYellow = rgb(0xf7c948)
	faceInk    = rgb(0x5b3a1a)
	tearBlue   = rgb(0x5aa9e6)
	heartRed   = rgb(0xe5484d)
	handTone   = rgb(0xf2b56b)
)

const faceCircle = `<circle cx="12" cy="12" r="10"/>`

var reactionArt = map[string][]artPart{
	"laugh": {
		{faceCircle, faceYellow, 0},
		{`<path d="M6.5 9.5q1.6-2 3.2 0M14.3 9.5q1.6-2 3.2 0"/>`, faceInk, 1.6},
		{`<path d="M6.5 13h11q-.8 5.5-5.5 5.5T6.5 13Z"/>`, faceInk, 0},
		{`<path d="M4 11.5q-1.5 2 0 3.5 1.5-1.5 0-3.5ZM20 11.5q1.5 2 0 3.5-1.5-1.5 0-3.5Z"/>`, tearBlue, 0},
	},
	"wow": {
		{faceCircle, faceYellow, 0},
		{`<circle cx="8.5" cy="10" r="1.6"/><circle cx="15.5" cy="10" r="1.6"/><ellipse cx="12" cy="16" rx="2.2" ry="2.8"/>`, faceInk, 0},
		{`<path d="M12 .5v2.5M5 2.5l1.5 2M19 2.5l-1.5 2"/>`, rgb(0xff9f43), 1.6},
	},
	"clown": {
		{faceCircle, rgb(0xfaf3e8), 0},
		{`<path d="M3 8q-2-4 2-5 0 3 2 4M21 8q2-4-2-5 0 3-2 4"/>`, rgb(0xe5484d), 0},
		{`<circle cx="8.5" cy="10" r="1.5"/><circle cx="15.5" cy="10" r="1.5"/>`, rgb(0x3b6fd8), 0},
		{`<circle cx="12" cy="13" r="2.2"/>`, rgb(0xe5484d), 0},
		{`<path d="M7.5 16.5q4.5 3.5 9 0"/>`, rgb(0xe5484d), 1.8},
	},
	"eyeroll": {
		{faceCircle, faceYellow, 0},
		{`<circle cx="8.5" cy="10" r="2.6"/><circle cx="15.5" cy="10" r="2.6"/>`, rgb(0xffffff), 0},
		{`<circle cx="8.5" cy="8.6" r="1.1"/><circle cx="15.5" cy="8.6" r="1.1"/>`, faceInk, 0},
		{`<path d="M8.5 16.5h7"/>`, faceInk, 1.6},
	},
	"sleep": {
		{faceCircle, faceYellow, 0},
		{`<path d="M6.5 10.5q1.6 1.5 3.2 0M14.3 10.5q1.6 1.5 3.2 0"/>`, faceInk, 1.6},
		{`<ellipse cx="12" cy="16" rx="1.6" ry="1.2"/>`, faceInk, 0},
		{`<path d="M17 2h4l-4 4h4"/>`, rgb(0x8ea3b8), 1.4},
	},
	"eyes": {
		{`<ellipse cx="7.5" cy="12" rx="5" ry="7"/><ellipse cx="16.5" cy="12" rx="5" ry="7"/>`, rgb(0xffffff), 0},
		{`<ellipse cx="7.5" cy="12" rx="5" ry="7"/><ellipse cx="16.5" cy="12" rx="5" ry="7"/>`, rgb(0x5f6976), 1.2},
		{`<circle cx="9" cy="12" r="2.4"/><circle cx="18" cy="12" r="2.4"/>`, rgb(0x29374a), 0},
	},
	"heart": {
		{`<path d="M12 21C3 15 1 9.5 4.5 6c2.5-2.4 5.5-1.6 7.5 1 2-2.6 5-3.4 7.5-1 3.5 3.5 1.5 9-7.5 15Z"/>`, heartRed, 0},
	},
	"star": {
		{`<path d="m12 2 2.9 6.3 6.9.7-5.2 4.6 1.5 6.8L12 16.9l-6.1 3.5 1.5-6.8L2.2 9l6.9-.7Z"/>`, rgb(0xf5b301), 0},
	},
	"fire": {
		{`<path d="M12 22c-4.5 0-7-3-7-6.5 0-4 3-6 3.5-10 2 1.5 3 3.5 3 5.5 1-1.5 1.5-3.5 1-6 4 2.5 6.5 6.5 6.5 10.5 0 3.5-2.5 6.5-7 6.5Z"/>`, rgb(0xff7a1a), 0},
		{`<path d="M12 22c-2.2 0-3.5-1.5-3.5-3.3 0-2 1.5-3 2-5 1.5 1 2.2 2 2.2 3.3.6-.8 1-1.8.8-3 2 1.3 2.5 3 2.5 4.7 0 1.8-1.5 3.3-4 3.3Z"/>`, rgb(0xffd23f), 0},
	},
	"thumbsup": {
		{`<path d="M3 11h3.5v10H3z"/>`, rgb(0x3b6fd8), 0},
		{`<path d="M7.5 11 11 3.5c1.6 0 2.5 1 2.2 2.8L12.6 10H19c1.4 0 2.3 1.3 1.9 2.6l-2.1 6.7c-.3 1-1.1 1.7-2.2 1.7H7.5Z"/>`, handTone, 0},
	},
	"thumbsdown": {
		{`<path d="M3 3h3.5v10H3z"/>`, rgb(0x3b6fd8), 0},
		{`<path d="M7.5 13 11 20.5c1.6 0 2.5-1 2.2-2.8L12.6 14H19c1.4 0 2.3-1.3 1.9-2.6l-2.1-6.7C18.5 3.7 17.7 3 16.6 3H7.5Z"/>`, handTone, 0},
	},
	"wave": {
		{`<path d="M7 21c-3-2-4.5-5.5-3.5-8l2-4c.5-1 2-.8 2 .4l-.5 3.4L11 4.5c.5-1.2 2.3-.8 2.2.5l-1.6 6.3 3.8-7.8c.6-1.1 2.2-.6 2 .6l-2.6 7.6 3-5c.7-1 2.2-.4 1.9.8l-3 8.5c-1.4 4.2-5.5 6.2-9.7 4Z"/>`, handTone, 0},
		{`<path d="M18.5 15q2 1 2 3.5M2 6.5q1-2 3-2.5"/>`, rgb(0x8ea3b8), 1.4},
	},
	"clap": {
		{`<path d="M5 20c-2.5-2.5-2.5-6 0-8.5l5-5c.8-.8 2-.2 1.8.9L10 11l4-4c.8-.8 2 0 1.6 1l-3 4.3 3.4-2.8c.9-.7 2 .4 1.3 1.3L12.5 17c-2.2 3-5.5 4.8-7.5 3Z"/>`, handTone, 0},
		{`<path d="M16 3.5 15 1.5M19 5l1.8-1.3M20 8.3h2.2"/>`, rgb(0xf5b301), 1.5},
	},
	"party": {
		{`<path d="M3 21 8 7l9 9Z"/>`, rgb(0x7c5cff), 0},
		{`<path d="M5.5 14.5l3 3M6.8 10.8l5.4 5.4"/>`, rgb(0xf7c948), 1.6},
		{`<circle cx="15" cy="4" r="1.3"/><circle cx="20" cy="9" r="1.3"/><rect x="17.5" y="2" width="2" height="3" rx=".6"/>`, rgb(0xe5484d), 0},
		{`<path d="M12 6q2-2 1-4M18 12q2-1 4 0"/>`, rgb(0x2fa66a), 1.4},
	},
}

var tomatoArt = []artPart{
	{`<path d="M20 11C5 3 0 31 16 34c20 5 26-30 4-23Z"/>`, rgb(0xe96c55), 0},
	{`<path d="m20 14-9-6 8 2 3-6 2 6 8-1-7 6Z"/>`, rgb(0x448e6b), 0},
	{`<path d="M10 20q-2 5 1 7"/>`, rgb(0xffb399), 3},
}

var poopArt = []artPart{
	{`<path d="M24 5c2 7 10 7 5 13 8 1 9 7 5 10 8 8-22 9-28 3-5-5 1-9 5-10-4-5 2-8 6-9 4-1 5-4 7-7Z"/>`, rgb(0x997554), 0},
	{`<path d="M13 21q9 3 17-1M9 29q12 3 22-1"/>`, rgb(0x795b43), 2},
	{`<circle cx="18" cy="23" r="1.5"/><circle cx="25" cy="23" r="1.5"/>`, rgb(0xfff4dd), 0},
}

type parsedPart struct {
	shapes []vshape
	color  color.NRGBA
	stroke float32
}

var reactionShapes = sync.OnceValue(func() map[string][]parsedPart {
	out := map[string][]parsedPart{}
	parse := func(id string, parts []artPart) {
		for _, part := range parts {
			shapes, err := parseFragment(part.svg, part.stroke == 0)
			if err != nil {
				continue
			}
			out[id] = append(out[id], parsedPart{shapes, part.color, part.stroke})
		}
	}
	for id, parts := range reactionArt {
		parse(id, parts)
	}
	parse("tomato", tomatoArt)
	parse("poop", poopArt)
	return out
})

// reactionIcon draws a reaction side pixels wide.
func reactionIcon(gtx layout.Context, id string, side float32) layout.Dimensions {
	view := float32(24)
	if id == "tomato" || id == "poop" {
		view = 40
	}
	size := px(gtx, side)
	scale := size / view
	for _, part := range reactionShapes()[id] {
		if part.stroke > 0 {
			roundStroke(gtx.Ops, part.shapes, scale, f32.Point{}, part.stroke, part.color)
		} else {
			fillShapes(gtx.Ops, part.shapes, scale, f32.Point{}, part.color)
		}
	}
	n := int(size + 0.5)
	return layout.Dimensions{Size: image.Pt(n, n)}
}
