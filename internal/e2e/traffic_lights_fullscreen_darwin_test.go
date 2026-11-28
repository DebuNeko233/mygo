//go:build darwin && !ios

/*
Checks AppKit's ownership of traffic lights throughout full-screen transitions
and restores the requested windowed placement after native animations finish.
*/

package e2e

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/darwin"
	"github.com/egoist/mygo/internal/wait"
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
				if strings.Contains(tc.begin, "WillExit") && buttonVisible(w) {
					t.Error("custom buttons exposed AppKit's temporary exit position")
				}
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
			eventually(t, "temporary traffic-light hiding restored", func() bool {
				var visible bool
				mygo.RunOnMain(func() { visible = buttonVisible(w) })
				return visible
			})
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
				opts := mygo.WindowOptions{Width: 480, Height: 320, Hidden: true, TitleBarStyle: style,
					TrafficLightPosition: &mygo.Point{X: 20, Y: 20}}
				if native {
					opts.Content = ui.View(func(c *ui.Context) {
						ui.Column(c).Fill().Padding(60, 20).Children(func() { ui.Text(c, "Full-screen controls") })
					})
				} else {
					opts.URL = "app://localhost/"
				}
				var w *mygo.Window
				ready := make(chan struct{}, 1)
				mygo.RunOnMain(func() {
					w = newWindow(t, opts)
					t.Cleanup(w.OnReadyToShow(func() { ready <- struct{}{} }))
				})
				var entered, left atomic.Bool
				events := observeFullScreen(t, w)
				t.Cleanup(w.OnEnterFullScreen(func() { entered.Store(true) }))
				t.Cleanup(w.OnLeaveFullScreen(func() { left.Store(true) }))
				placed := func(when string) {
					t.Helper()
					x, y, _ := trafficLights(w)
					if x != 20 || y != 20 {
						t.Errorf("%s: controls at (%g,%g), want (20,20)", when, x, y)
					}
				}
				// Subscribe before native drawing or page loading can finish.
				wait.For(t, "window ready to show", ready)
				showForFullScreen(t, w)
				placed("before full screen")
				w.SetFullScreen(true)
				waitFullScreen(t, events, true)
				if !entered.Load() {
					t.Fatal("native entry did not emit the public callback")
				}
				if !w.IsFullScreen() {
					t.Fatal("entered event left the window outside full screen")
				}
				var exposed atomic.Bool
				t.Cleanup(w.OnResize(func() {
					// AppKit clears the style bit before the exit animation
					// ends. None of that intermediate title bar may show.
					if !left.Load() && !w.IsFullScreen() && buttonVisible(w) {
						exposed.Store(true)
					}
				}))
				w.SetFullScreen(false)
				waitFullScreen(t, events, false)
				if !left.Load() {
					t.Fatal("native exit did not emit the public callback")
				}
				if w.IsFullScreen() {
					t.Fatal("left event retained the full-screen style")
				}
				eventually(t, "custom traffic lights visible at their restored position", func() bool {
					var visible bool
					var x, y float64
					mygo.RunOnMain(func() {
						visible = buttonVisible(w)
						x, y = darwin.TestTrafficLights(w.NativeHandle())
					})
					return visible && x == 20 && y == 20
				})
				if exposed.Load() {
					t.Error("native exit exposed custom controls before restoring their position")
				}
				placed("after full-screen exit")
				w.SetSize(520, 360)
				placed("after resizing the restored window")
			})
		}
	}
}

// Read on the main thread; no runtime hooks are needed for native visibility.
func buttonVisible(w *mygo.Window) bool {
	win := objc.ID(w.NativeHandle())
	for i := uintptr(0); i < 3; i++ {
		button := win.Send(objc.RegisterName("standardWindowButton:"), i)
		if button != 0 && button.Send(objc.RegisterName("isHiddenOrHasHiddenAncestor")) == 0 {
			return true
		}
	}
	return false
}

// Deferred restoration must not override a new transition or a caller's choice
// to hide controls. Sending native notifications keeps cancellation deterministic.
func TestTrafficLightsDeferredRestoreLifecycle(t *testing.T) {
	for _, hide := range []bool{false, true} {
		t.Run(fmt.Sprintf("hidden=%v", hide), func(t *testing.T) {
			w := newWindow(t, mygo.WindowOptions{Width: 440, Height: 280,
				TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: hide,
				TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
				Content:              ui.View(func(c *ui.Context) { ui.Text(c, "Transition cancellation") })})
			notify := func(name string) {
				delegate := objc.ID(w.NativeHandle()).Send(objc.RegisterName("delegate"))
				delegate.Send(objc.RegisterName(name), objc.ID(0))
			}
			mygo.RunOnMain(func() {
				notify("windowWillExitFullScreen:")
				notify("windowDidExitFullScreen:")
				notify("windowWillEnterFullScreen:")
			})
			// This main-thread barrier runs after the deferred restoration.
			mygo.RunOnMain(func() {
				if buttonVisible(w) {
					t.Error("queued restore exposed buttons during a new transition")
				}
				notify("windowDidFailToEnterFullScreen:")
			})
			eventually(t, "cancelled transition restored requested visibility", func() bool {
				var visible bool
				mygo.RunOnMain(func() { visible = buttonVisible(w) })
				return visible == !hide
			})
			mygo.RunOnMain(func() {
				notify("windowWillExitFullScreen:")
				notify("windowDidExitFullScreen:")
				w.Close() // its deferred restoration must not touch native objects
			})
			mygo.RunOnMain(func() {})
		})
	}
}
