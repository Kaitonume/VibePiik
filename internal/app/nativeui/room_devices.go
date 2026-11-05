package nativeui

import (
	"context"
	"image"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// deviceList is a microphone list being loaded for one chooser.
type deviceList struct {
	devices []audioDevice
	busy    bool
	failed  bool
	loaded  bool
}

// devicePicker is MicrophoneDeviceSelect: "Default device", each microphone
// and a refresh button, as chips.
type devicePicker struct {
	none    Chip
	missing Chip
	chips   []Chip
	refresh Button
}

func (d *devicePicker) Layout(gtx layout.Context, u *UI, list deviceList, current string) (layout.Dimensions, bool, string, bool) {
	changed, chosen, refresh := false, current, false
	if d.none.Clicked(gtx) && current != "" {
		changed, chosen = true, ""
	}
	for len(d.chips) < len(list.devices) {
		d.chips = append(d.chips, Chip{})
	}
	for i, device := range list.devices {
		if d.chips[i].Clicked(gtx) && device.ID != current {
			changed, chosen = true, device.ID
		}
	}
	if d.refresh.Clicked(gtx) && !list.busy {
		refresh = true
	}
	missing := current != ""
	for _, device := range list.devices {
		if device.ID == current {
			missing = false
		}
	}
	if !list.loaded {
		missing = false
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			items := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
				return d.none.Layout(gtx, u, "microphone", u.T("host.device.default"), "", current == "", false)
			}}
			if missing {
				items = append(items, func(gtx layout.Context) layout.Dimensions {
					return d.missing.Layout(gtx, u, "alert", u.T("host.device.missing"), "", true, true)
				})
			}
			for i, device := range list.devices {
				items = append(items, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(280))
					label := device.Label
					if label == "" {
						label = u.T("host.device.microphone")
					}
					return d.chips[i].Layout(gtx, u, "", label, "", device.ID == current, false)
				})
			}
			items = append(items, func(gtx layout.Context) layout.Dimensions {
				return d.refresh.Layout(gtx, u, ButtonStyle{Icon: "refresh", Size: sizeDense, Busy: list.busy, Disabled: list.busy,
					Tip: u.T("host.device.refresh")})
			})
			return flowRow(gtx, 8, items)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !list.failed || u.Visual() {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.T("host.device.listFailed"), textStyle{size: 11.5, color: u.P.DangerText})
			})
		}))
	return dims, changed, chosen, refresh
}

// needDevices reports a list never loaded nor loading.
func (r *Room) needDevices(list *deviceList) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !list.loaded && !list.busy
}

// loadDevices fills a device list in the background.
func (r *Room) loadDevices(list *deviceList, load func() ([]audioDevice, error)) {
	r.mu.Lock()
	if list.busy {
		r.mu.Unlock()
		return
	}
	list.busy = true
	r.mu.Unlock()
	r.invalidate()
	go func() {
		devices, err := load()
		r.mu.Lock()
		list.busy, list.failed, list.loaded = false, err != nil, true
		if err == nil {
			list.devices = devices
		}
		r.mu.Unlock()
		r.invalidate()
	}()
}

func (r *Room) commentaryMicrophones() ([]audioDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return r.host.Microphones(ctx)
}

// volumeRow is a 0–200 % volume slider with its value; level (0–1, <0 for
// none) draws a live microphone meter under it (MicrophoneLevel).
func volumeRow(gtx layout.Context, u *UI, slider *Slider, icon, caption string, volume float64, level float32, set func(float64)) layout.Dimensions {
	p := &u.P
	percent := int(math.Round(volume * 100))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return heading(gtx, u, icon, caption) }),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return valueBox(gtx, u, itoa(percent), "%") }))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d, value, moved := slider.Layout(gtx, u, float32(percent), 0, 200, 5)
			if moved && int(value) != percent {
				set(float64(value) / 100)
			}
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if level < 0 {
				return layout.Dimensions{}
			}
			width := gtx.Constraints.Max.X - gtx.Dp(18)
			size := image.Pt(width, gtx.Dp(6))
			track := boxOf(size, px(gtx, 3))
			defer op.Offset(image.Pt(gtx.Dp(9), gtx.Dp(2))).Push(gtx.Ops).Pop()
			fillBox(gtx.Ops, track, p.Wall2)
			// RMS of speech sits around 0.02–0.3: show it on a log-like scale.
			fill := float32(math.Min(1, math.Sqrt(float64(level))*1.6))
			if fill > 0.01 {
				bar := track
				bar.rect.Max.X = bar.rect.Min.X + fill*float32(width)
				color := p.Live
				if fill > 0.9 {
					color = p.Warn
				}
				fillBox(gtx.Ops, bar, color)
			}
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(80 * time.Millisecond)})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(10))}
		}))
}
