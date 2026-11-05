package nativeui

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// segment is one absolute path command in viewBox units: 'M', 'L', 'C', 'Q' or 'Z'.
type segment struct {
	op  byte
	pts [3]f32.Point
}

// vshape is one drawable element of a vector picture.
type vshape struct {
	segs        []segment
	fill        bool // filled instead of stroked
	strokeWidth float32
	opacity     float32
	dots        []f32.Point // zero-length subpaths: drawn as round dots
}

type glyphSource struct {
	solid bool
	svg   string
}

type svgElement struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
}

type svgGroup struct {
	Elements []svgElement `xml:",any"`
}

// parseFragment reads the <path>, <circle>, <rect>, <ellipse> and <line>
// elements of an SVG fragment. solid fills every shape.
func parseFragment(fragment string, solid bool) ([]vshape, error) {
	var group svgGroup
	if err := xml.Unmarshal([]byte("<g>"+fragment+"</g>"), &group); err != nil {
		return nil, err
	}
	shapes := make([]vshape, 0, len(group.Elements))
	for _, element := range group.Elements {
		attr := map[string]string{}
		for _, a := range element.Attrs {
			attr[a.Name.Local] = a.Value
		}
		number := func(name string) float32 {
			value, _ := strconv.ParseFloat(attr[name], 32)
			return float32(value)
		}
		shape := vshape{fill: solid, opacity: 1}
		if attr["fill"] == "currentColor" {
			shape.fill = true
		}
		if attr["stroke"] == "none" {
			shape.fill = true
		}
		if value, ok := attr["strokeWidth"]; ok {
			width, _ := strconv.ParseFloat(value, 32)
			shape.strokeWidth = float32(width)
		}
		if value, ok := attr["opacity"]; ok {
			opacity, _ := strconv.ParseFloat(value, 32)
			shape.opacity = float32(opacity)
		}
		switch element.XMLName.Local {
		case "path":
			segs, err := parsePathData(attr["d"])
			if err != nil {
				return nil, err
			}
			shape.segs = segs
		case "circle":
			shape.segs = ellipseSegments(number("cx"), number("cy"), number("r"), number("r"))
		case "ellipse":
			shape.segs = ellipseSegments(number("cx"), number("cy"), number("rx"), number("ry"))
		case "rect":
			shape.segs = roundRectSegments(number("x"), number("y"), number("width"), number("height"), number("rx"))
		case "line":
			shape.segs = []segment{{op: 'M', pts: [3]f32.Point{{X: number("x1"), Y: number("y1")}}},
				{op: 'L', pts: [3]f32.Point{{X: number("x2"), Y: number("y2")}}}}
		default:
			return nil, fmt.Errorf("unsupported SVG element %q", element.XMLName.Local)
		}
		shape.segs, shape.dots = splitDots(shape.segs)
		shapes = append(shapes, shape)
	}
	return shapes, nil
}

// splitDots moves subpaths that do not move (like "M12 16.5h.01") out of the
// outline: a stroked dot is a filled circle the size of the stroke.
func splitDots(segs []segment) ([]segment, []f32.Point) {
	var kept []segment
	var dots []f32.Point
	for start := 0; start < len(segs); {
		end := start + 1
		for end < len(segs) && segs[end].op != 'M' {
			end++
		}
		sub := segs[start:end]
		extent := float32(0)
		origin := sub[0].pts[0]
		for _, s := range sub[1:] {
			count := map[byte]int{'L': 1, 'Q': 2, 'C': 3}[s.op]
			for _, p := range s.pts[:count] {
				extent = max(extent, abs32(p.X-origin.X), abs32(p.Y-origin.Y))
			}
		}
		if len(sub) > 1 && extent < 0.05 {
			dots = append(dots, origin)
		} else {
			kept = append(kept, sub...)
		}
		start = end
	}
	return kept, dots
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

const kappa = 0.5522847498

func ellipseSegments(cx, cy, rx, ry float32) []segment {
	kx, ky := rx*kappa, ry*kappa
	p := func(x, y float32) f32.Point { return f32.Pt(cx+x, cy+y) }
	return []segment{
		{op: 'M', pts: [3]f32.Point{p(rx, 0)}},
		{op: 'C', pts: [3]f32.Point{p(rx, ky), p(kx, ry), p(0, ry)}},
		{op: 'C', pts: [3]f32.Point{p(-kx, ry), p(-rx, ky), p(-rx, 0)}},
		{op: 'C', pts: [3]f32.Point{p(-rx, -ky), p(-kx, -ry), p(0, -ry)}},
		{op: 'C', pts: [3]f32.Point{p(kx, -ry), p(rx, -ky), p(rx, 0)}},
		{op: 'Z'},
	}
}

func roundRectSegments(x, y, w, h, r float32) []segment {
	r = min(r, w/2, h/2)
	if r <= 0 {
		return []segment{
			{op: 'M', pts: [3]f32.Point{f32.Pt(x, y)}}, {op: 'L', pts: [3]f32.Point{f32.Pt(x+w, y)}},
			{op: 'L', pts: [3]f32.Point{f32.Pt(x+w, y+h)}}, {op: 'L', pts: [3]f32.Point{f32.Pt(x, y+h)}}, {op: 'Z'},
		}
	}
	k := r * kappa
	return []segment{
		{op: 'M', pts: [3]f32.Point{f32.Pt(x+r, y)}},
		{op: 'L', pts: [3]f32.Point{f32.Pt(x+w-r, y)}},
		{op: 'C', pts: [3]f32.Point{f32.Pt(x+w-r+k, y), f32.Pt(x+w, y+r-k), f32.Pt(x+w, y+r)}},
		{op: 'L', pts: [3]f32.Point{f32.Pt(x+w, y+h-r)}},
		{op: 'C', pts: [3]f32.Point{f32.Pt(x+w, y+h-r+k), f32.Pt(x+w-r+k, y+h), f32.Pt(x+w-r, y+h)}},
		{op: 'L', pts: [3]f32.Point{f32.Pt(x+r, y+h)}},
		{op: 'C', pts: [3]f32.Point{f32.Pt(x+r-k, y+h), f32.Pt(x, y+h-r+k), f32.Pt(x, y+h-r)}},
		{op: 'L', pts: [3]f32.Point{f32.Pt(x, y+r)}},
		{op: 'C', pts: [3]f32.Point{f32.Pt(x, y+r-k), f32.Pt(x+r-k, y), f32.Pt(x+r, y)}},
		{op: 'Z'},
	}
}

// parsePathData converts SVG path data to absolute segments. Arcs become cubic
// curves; smooth curves get their reflected control points.
func parsePathData(d string) ([]segment, error) {
	tokens := tokenizePath(d)
	var segs []segment
	var current, start, lastControl f32.Point
	var lastOp byte
	index := 0
	next := func() (float32, error) {
		if index >= len(tokens) || isCommand(tokens[index]) {
			return 0, fmt.Errorf("path %q: missing number", d)
		}
		value, err := strconv.ParseFloat(tokens[index], 32)
		index++
		return float32(value), err
	}
	var command byte
	for index < len(tokens) {
		if isCommand(tokens[index]) {
			command = tokens[index][0]
			index++
		} else if command == 0 {
			return nil, fmt.Errorf("path %q: number before command", d)
		}
		relative := command >= 'a' && command <= 'z'
		offset := f32.Point{}
		if relative {
			offset = current
		}
		upper := command &^ 0x20
		switch upper {
		case 'Z':
			segs = append(segs, segment{op: 'Z'})
			current = start
			lastOp = 'Z'
			continue
		case 'M', 'L':
			x, err := next()
			if err != nil {
				return nil, err
			}
			y, err := next()
			if err != nil {
				return nil, err
			}
			p := f32.Pt(x, y).Add(offset)
			if upper == 'M' {
				segs = append(segs, segment{op: 'M', pts: [3]f32.Point{p}})
				start = p
				// Further coordinate pairs after M are implicit L commands.
				if relative {
					command = 'l'
				} else {
					command = 'L'
				}
			} else {
				segs = append(segs, segment{op: 'L', pts: [3]f32.Point{p}})
			}
			current = p
		case 'H', 'V':
			v, err := next()
			if err != nil {
				return nil, err
			}
			p := current
			if upper == 'H' {
				p.X = v
				if relative {
					p.X = current.X + v
				}
			} else {
				p.Y = v
				if relative {
					p.Y = current.Y + v
				}
			}
			segs = append(segs, segment{op: 'L', pts: [3]f32.Point{p}})
			current = p
		case 'C', 'S', 'Q', 'T':
			var values []float32
			count := map[byte]int{'C': 6, 'S': 4, 'Q': 4, 'T': 2}[upper]
			for i := 0; i < count; i++ {
				v, err := next()
				if err != nil {
					return nil, err
				}
				values = append(values, v)
			}
			pt := func(i int) f32.Point { return f32.Pt(values[i], values[i+1]).Add(offset) }
			reflected := current
			if (upper == 'S' && (lastOp == 'C' || lastOp == 'S')) || (upper == 'T' && (lastOp == 'Q' || lastOp == 'T')) {
				reflected = current.Mul(2).Sub(lastControl)
			}
			switch upper {
			case 'C':
				segs = append(segs, segment{op: 'C', pts: [3]f32.Point{pt(0), pt(2), pt(4)}})
				lastControl, current = pt(2), pt(4)
			case 'S':
				segs = append(segs, segment{op: 'C', pts: [3]f32.Point{reflected, pt(0), pt(2)}})
				lastControl, current = pt(0), pt(2)
			case 'Q':
				segs = append(segs, segment{op: 'Q', pts: [3]f32.Point{pt(0), pt(2)}})
				lastControl, current = pt(0), pt(2)
			case 'T':
				segs = append(segs, segment{op: 'Q', pts: [3]f32.Point{reflected, pt(0)}})
				lastControl, current = reflected, pt(0)
			}
			lastOp = upper
			continue
		case 'A':
			var values [7]float32
			for i := range values {
				v, err := next()
				if err != nil {
					return nil, err
				}
				values[i] = v
			}
			end := f32.Pt(values[5], values[6]).Add(offset)
			segs = append(segs, arcSegments(current, end, values[0], values[1], values[2], values[3] != 0, values[4] != 0)...)
			current = end
		default:
			return nil, fmt.Errorf("path %q: unsupported command %c", d, command)
		}
		lastOp = upper
	}
	return segs, nil
}

func isCommand(token string) bool {
	return len(token) == 1 && strings.ContainsRune("MmLlHhVvCcSsQqTtAaZz", rune(token[0]))
}

// tokenizePath splits path data into commands and numbers, including the
// compact forms "-.5.5" and arc flags written without separators ("a1 1 0 01").
func tokenizePath(d string) []string {
	var tokens []string
	var number strings.Builder
	arcArgument := -1 // position inside the current arc's 7 arguments
	var command byte
	flush := func() {
		if number.Len() > 0 {
			tokens = append(tokens, number.String())
			number.Reset()
			if arcArgument >= 0 {
				arcArgument = (arcArgument + 1) % 7
			}
		}
	}
	for i := 0; i < len(d); i++ {
		c := d[i]
		switch {
		case strings.IndexByte("MmLlHhVvCcSsQqTtAaZz", c) >= 0:
			flush()
			tokens = append(tokens, string(c))
			command = c &^ 0x20
			arcArgument = -1
			if command == 'A' {
				arcArgument = 0
			}
		case c == ' ' || c == ',' || c == '\n' || c == '\t':
			flush()
		case (arcArgument == 3 || arcArgument == 4) && (c == '0' || c == '1') && number.Len() == 0:
			// Arc flags are single digits.
			number.WriteByte(c)
			flush()
		case c == '-' || c == '+':
			if number.Len() > 0 && !strings.HasSuffix(number.String(), "e") {
				flush()
			}
			number.WriteByte(c)
		case c == '.':
			if strings.Contains(number.String(), ".") {
				flush()
			}
			number.WriteByte(c)
		default:
			number.WriteByte(c)
		}
	}
	flush()
	return tokens
}

// arcSegments converts an SVG elliptical arc to cubic curves (SVG 1.1 F.6).
func arcSegments(from, to f32.Point, rx, ry, rotation float32, large, sweep bool) []segment {
	if from == to {
		return nil
	}
	if rx == 0 || ry == 0 {
		return []segment{{op: 'L', pts: [3]f32.Point{to}}}
	}
	phi := float64(rotation) * math.Pi / 180
	cosPhi, sinPhi := math.Cos(phi), math.Sin(phi)
	dx, dy := float64(from.X-to.X)/2, float64(from.Y-to.Y)/2
	x1 := cosPhi*dx + sinPhi*dy
	y1 := -sinPhi*dx + cosPhi*dy
	a, b := math.Abs(float64(rx)), math.Abs(float64(ry))
	if lambda := x1*x1/(a*a) + y1*y1/(b*b); lambda > 1 {
		a, b = a*math.Sqrt(lambda), b*math.Sqrt(lambda)
	}
	numerator := a*a*b*b - a*a*y1*y1 - b*b*x1*x1
	denominator := a*a*y1*y1 + b*b*x1*x1
	coefficient := 0.0
	if denominator != 0 && numerator > 0 {
		coefficient = math.Sqrt(numerator / denominator)
	}
	if large == sweep {
		coefficient = -coefficient
	}
	cx1, cy1 := coefficient*a*y1/b, -coefficient*b*x1/a
	cx := cosPhi*cx1 - sinPhi*cy1 + float64(from.X+to.X)/2
	cy := sinPhi*cx1 + cosPhi*cy1 + float64(from.Y+to.Y)/2
	angle := func(ux, uy, vx, vy float64) float64 {
		return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	}
	theta := angle(1, 0, (x1-cx1)/a, (y1-cy1)/b)
	delta := angle((x1-cx1)/a, (y1-cy1)/b, (-x1-cx1)/a, (-y1-cy1)/b)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	} else if sweep && delta < 0 {
		delta += 2 * math.Pi
	}
	parts := int(math.Ceil(math.Abs(delta) / (math.Pi / 2)))
	step := delta / float64(parts)
	t := 4.0 / 3.0 * math.Tan(step/4)
	point := func(angle float64) (float64, float64) {
		x, y := a*math.Cos(angle), b*math.Sin(angle)
		return cosPhi*x - sinPhi*y + cx, sinPhi*x + cosPhi*y + cy
	}
	derivative := func(angle float64) (float64, float64) {
		x, y := -a*math.Sin(angle), b*math.Cos(angle)
		return cosPhi*x - sinPhi*y, sinPhi*x + cosPhi*y
	}
	segs := make([]segment, 0, parts)
	for i := 0; i < parts; i++ {
		a0, a1 := theta+float64(i)*step, theta+float64(i+1)*step
		x0, y0 := point(a0)
		x3, y3 := point(a1)
		d0x, d0y := derivative(a0)
		d1x, d1y := derivative(a1)
		end := f32.Pt(float32(x3), float32(y3))
		if i == parts-1 {
			end = to
		}
		segs = append(segs, segment{op: 'C', pts: [3]f32.Point{
			f32.Pt(float32(x0+t*d0x), float32(y0+t*d0y)),
			f32.Pt(float32(x3-t*d1x), float32(y3-t*d1y)),
			end,
		}})
	}
	return segs
}

// pathSpec builds the Gio outline of segs scaled by scale.
func pathSpec(ops *op.Ops, segs []segment, scale float32, origin f32.Point) clip.PathSpec {
	var path clip.Path
	path.Begin(ops)
	at := func(p f32.Point) f32.Point { return p.Mul(scale).Add(origin) }
	for _, s := range segs {
		switch s.op {
		case 'M':
			path.MoveTo(at(s.pts[0]))
		case 'L':
			path.LineTo(at(s.pts[0]))
		case 'C':
			path.CubeTo(at(s.pts[0]), at(s.pts[1]), at(s.pts[2]))
		case 'Q':
			path.QuadTo(at(s.pts[0]), at(s.pts[1]))
		case 'Z':
			path.Close()
		}
	}
	return path.End()
}
