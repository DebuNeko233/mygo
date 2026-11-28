//go:build darwin && !ios

package e2e

import (
	"fmt"
	"sync"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
)

// AppKit can key a window before the Space containing it has appeared. A
// full-screen request meanwhile fails, although IsVisible and IsFocused hold.
func showForFullScreen(t *testing.T, w *mygo.Window) {
	t.Helper()
	mygo.App.Show()
	w.Show() // activation needs a visible window when leaving another app's Space
	mygo.App.Focus()
	eventually(t, "application active before window focus", mygo.App.IsActive)
	w.Focus()
	eventually(t, "focused window presented on the active Space", func() bool {
		var presented bool
		mygo.RunOnMain(func() {
			win := objc.ID(w.NativeHandle())
			const visible = 1 << 1 // NSWindowOcclusionStateVisible
			presented = mygo.App.IsActive() && w.IsFocused() && w.IsVisible() &&
				win.Send(objc.RegisterName("isOnActiveSpace")) != 0 &&
				win.Send(objc.RegisterName("occlusionState"))&visible != 0
		})
		return presented
	})
}

// A native failure is a completed, rejected transition, not a missing success
// callback. Observe all four outcomes without changing the backend or its
// handler. The subclass has no ivars; the same delegate still forwards to the
// production implementation. Its callbacks are allocated once per test process.
var fullScreenObservers = map[objc.ID]chan objc.SEL{} // main thread only

var fullScreenOutcomes = []string{
	"windowDidEnterFullScreen:", "windowDidExitFullScreen:",
	"windowDidFailToEnterFullScreen:", "windowDidFailToExitFullScreen:",
}

var installFullScreenObserverClass = sync.OnceValues(func() (func(objc.ID) func(), error) {
	lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	defer purego.Dlclose(lib) // objc itself keeps this library loaded
	var setClass func(objc.ID, objc.Class) objc.Class
	purego.RegisterLibFunc(&setClass, lib, "object_setClass")
	var methods []objc.MethodDef
	for _, name := range fullScreenOutcomes {
		methods = append(methods, objc.MethodDef{Cmd: objc.RegisterName(name), Fn: func(self objc.ID, cmd objc.SEL, arg objc.ID) {
			self.SendSuper(cmd, arg)
			if ch := fullScreenObservers[self]; ch != nil {
				select {
				case ch <- cmd:
				default:
					panic("full-screen test observation overflow")
				}
			}
		}})
	}
	cls, err := objc.RegisterClass("MyGoTestFullScreenDelegate", objc.GetClass("MyGoWindowDelegate"), nil, nil, methods)
	if err != nil {
		return nil, err
	}
	return func(delegate objc.ID) func() {
		previous := setClass(delegate, cls)
		return func() { setClass(delegate, previous) }
	}, nil
})

func observeFullScreen(t *testing.T, w *mygo.Window) <-chan objc.SEL {
	t.Helper()
	ch := make(chan objc.SEL, 8)
	mygo.RunOnMain(func() {
		install, err := installFullScreenObserverClass()
		if err != nil {
			panic(fmt.Sprintf("register full-screen test delegate: %v", err))
		}
		delegate := objc.ID(w.NativeHandle()).Send(objc.RegisterName("delegate"))
		restore := install(delegate)
		fullScreenObservers[delegate] = ch
		t.Cleanup(func() {
			mygo.RunOnMain(func() {
				delete(fullScreenObservers, delegate)
				restore()
			})
		})
	})
	return ch
}

func waitFullScreen(t *testing.T, events <-chan objc.SEL, enter bool) {
	t.Helper()
	want := "windowDidExitFullScreen:"
	if enter {
		want = "windowDidEnterFullScreen:"
	}
	if got := wait.For(t, want, events); got != objc.RegisterName(want) {
		name := fmt.Sprint(got)
		for _, outcome := range fullScreenOutcomes {
			if got == objc.RegisterName(outcome) {
				name = outcome
			}
		}
		t.Fatalf("wanted %s, received %s (AppKit rejected or reversed the transition)", want, name)
	}
}

// A rejected transition reaches the observer without a success event; native
// cancellation still runs the backend's restoration and public success remains
// reserved for successful transitions.
func TestFullScreenFailureObservation(t *testing.T) {
	w := newWindow(t, mygo.WindowOptions{Hidden: true, TitleBarStyle: mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 20, Y: 20}})
	events := observeFullScreen(t, w)
	var successes int
	t.Cleanup(w.OnEnterFullScreen(func() { successes++ }))
	t.Cleanup(w.OnLeaveFullScreen(func() { successes++ }))
	for _, name := range []string{"windowDidFailToEnterFullScreen:", "windowDidFailToExitFullScreen:"} {
		cmd := objc.RegisterName(name)
		mygo.RunOnMain(func() {
			delegate := objc.ID(w.NativeHandle()).Send(objc.RegisterName("delegate"))
			begin := "windowWillEnterFullScreen:"
			if name == "windowDidFailToExitFullScreen:" {
				begin = "windowWillExitFullScreen:"
			}
			delegate.Send(objc.RegisterName(begin), objc.ID(0))
			delegate.Send(cmd, objc.ID(w.NativeHandle()))
		})
		if got := wait.For(t, "native failure observed", events); got != cmd {
			t.Fatalf("failure outcome %v, want %v", got, cmd)
		}
		eventually(t, "real delegate restored controls after rejection", func() bool {
			x, y, _ := trafficLights(w)
			var visible bool
			mygo.RunOnMain(func() { visible = buttonVisible(w) })
			return visible && x == 20 && y == 20
		})
	}
	mygo.RunOnMain(func() {
		if successes != 0 {
			t.Errorf("rejected transitions emitted %d success callbacks", successes)
		}
	})
}
