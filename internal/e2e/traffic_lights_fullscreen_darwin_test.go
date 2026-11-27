//go:build darwin && !ios

/*
Checks AppKit's ownership of traffic lights throughout full-screen transitions
and restores the requested windowed placement after native animations finish.
*/

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/ui"
)

// Resize notifications must not apply windowed coordinates while AppKit owns
// the controls, even when the full-screen style bit is already absent. Success
// and cancellation both release that ownership without leaving layout disabled.
func TestTrafficLightsRespectFullScreenTransition(t *testing.T) {
	for _, tc := range []struct{ name, begin, end string }{
		{"enter", "windowWillEnterFullScreen:", "windowDidEnterFullScreen:"},
		{"cancel enter", "windowWillEnterFullScreen:", "windowDidFailToEnterFullScreen:"},
		{"exit", "windowWillExitFullScreen:", "windowDidExitFullScreen:"},
		{"cancel exit", "windowWillExitFullScreen:", "windowDidFailToExitFullScreen:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWindow(t, mygo.WindowOptions{Width: 440, Height: 280,
				TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
				Content: ui.View(func(c *ui.Context) { ui.Text(c, "Full screen transition") })})
			var beforeX, beforeY, duringX, duringY, afterX, afterY float64
			mygo.RunOnMain(func() {
				win := objc.ID(w.NativeHandle())
				delegate := win.Send(objc.RegisterName("delegate"))
				notify := func(name string) {
					selector := objc.RegisterName(name)
					if delegate.Send(objc.RegisterName("respondsToSelector:"), selector) != 0 {
						delegate.Send(selector, objc.ID(0))
					}
				}
				notify(tc.begin)
				// Model a button position chosen by AppKit's intermediate title bar,
				// then deliver a resize while the native full-screen bit is clear.
				button := win.Send(objc.RegisterName("standardWindowButton:"), uintptr(0))
				frame := objc.Send[darwin.NSRect](button, objc.RegisterName("frame"))
				frame.Origin.X += 11
				frame.Origin.Y += 7
				button.Send(objc.RegisterName("setFrameOrigin:"), frame.Origin)
				beforeX, beforeY = darwin.TestTrafficLights(w.NativeHandle())
				notify("windowDidResize:")
				duringX, duringY = darwin.TestTrafficLights(w.NativeHandle())
				notify(tc.end)
				notify("windowDidResize:")
				afterX, afterY = darwin.TestTrafficLights(w.NativeHandle())
			})
			if duringX != beforeX || duringY != beforeY {
				t.Errorf("resize overwrote AppKit's transition position: (%g,%g) -> (%g,%g)", beforeX, beforeY, duringX, duringY)
			}
			if afterX != 20 || afterY != 20 {
				t.Errorf("windowed layout remained disabled after completion: (%g,%g)", afterX, afterY)
			}
		})
	}
}

// Real AppKit enter/exit animations restore custom controls for native UI and
// webviews, including inset title bars; later resizes keep the same placement.
func TestTrafficLightsFullScreenRoundTrip(t *testing.T) {
	for _, style := range []mygo.TitleBarStyle{mygo.TitleBarHidden, mygo.TitleBarHiddenInset} {
		for _, native := range []bool{true, false} {
			name := string(style) + "/webview"
			if native {
				name = string(style) + "/native"
			}
			t.Run(name, func(t *testing.T) {
				opts := mygo.WindowOptions{Width: 480, Height: 320, TitleBarStyle: style,
					TrafficLightPosition: &mygo.Point{X: 20, Y: 20}}
				if native {
					opts.Content = ui.View(func(c *ui.Context) {
						ui.Column(c).Fill().Padding(60, 20).Children(func() { ui.Text(c, "Full-screen controls") })
					})
				}
				w := newWindow(t, opts)
				var entered, left atomic.Bool
				t.Cleanup(w.OnEnterFullScreen(func() { entered.Store(true) }))
				t.Cleanup(w.OnLeaveFullScreen(func() { left.Store(true) }))
				placed := func(when string) {
					t.Helper()
					x, y, _ := trafficLights(w)
					if x != 20 || y != 20 {
						t.Errorf("%s: controls at (%g,%g), want (20,20)", when, x, y)
					}
				}
				placed("before full screen")
				w.Focus()
				w.SetFullScreen(true)
				eventually(t, "full-screen entrance completed", entered.Load)
				if !w.IsFullScreen() {
					t.Fatal("entered event left the window outside full screen")
				}
				w.SetFullScreen(false)
				eventually(t, "full-screen exit completed", left.Load)
				if w.IsFullScreen() {
					t.Fatal("left event retained the full-screen style")
				}
				placed("after full-screen exit")
				w.SetSize(520, 360)
				placed("after resizing the restored window")
			})
		}
	}
}
