/*
Checks that hidden view cleanup keeps the last native picture while preserving
resource cleanup, captures, the first frame and ordinary input-driven updates.
*/

package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// A hidden cleanup frame may release the view's graphics, but the window
// server can still be animating the last picture, as during full-screen changes.
type occludedBackgroundSurface struct {
	*occludedSurface
	backgrounds []platform.Color
}

func (s *occludedBackgroundSurface) SetBackground(c platform.Color) {
	s.backgrounds = append(s.backgrounds, c)
}

// Cleanup runs without replacing the visible surface, on CPU and GPU paths.
func TestHiddenCleanupKeepsPresentedPicture(t *testing.T) {
	for _, gpu := range []bool{false, true} {
		name := "software"
		if gpu {
			name = "GPU"
		}
		t.Run(name, func(t *testing.T) {
			o := newOccludedHost(t, false)
			s := &occludedBackgroundSurface{occludedSurface: o.s}
			o.conn.Surface = s
			g := &testGPU{}
			if gpu {
				t.Setenv("MYGO_GPU", "")
				previous := newGPU
				newGPU = func(platform.SurfaceNative) (gpuRenderer, error) { return g, nil }
				t.Cleanup(func() { newGPU = previous })
			}
			presents := func() int {
				if gpu {
					return g.frames
				}
				return s.pixels
			}
			cleanup := 0
			o.rt = newRuntime(func(c *context) {
				if c.WindowHidden() {
					cleanup++
					c.Root().Background(RGB(255, 255, 255))
					return
				}
				c.Root().Background(RGB(uint8(30+o.count), 60, 90))
				coreText(c, "Visible content")
			}, o.windowHost)
			o.frame()
			before, backgrounds := presents(), len(s.backgrounds)
			if before != 1 {
				t.Fatalf("first frame presented %d times", before)
			}
			s.occluded = true
			o.event(platform.SurfaceEvent{Kind: platform.SurfaceHidden})
			o.frame()
			if cleanup != 1 {
				t.Fatalf("hidden view did not clean up: %d", cleanup)
			}
			if presents() != before || len(s.backgrounds) != backgrounds {
				t.Fatalf("cleanup replaced displayed picture: presentations %d→%d, backgrounds %d→%d", before, presents(), backgrounds, len(s.backgrounds))
			}
			// Capturing hidden content must remain fresh without presenting it,
			// and repeated captures must not rebuild it.
			o.count++
			o.rt.changed()
			w, h, pix := o.capture()
			if w == 0 || h == 0 || len(pix) < 4 || pix[0] != 255 || pix[1] != 255 || pix[2] != 255 {
				t.Fatal("capture did not build the current hidden cleanup content")
			}
			cleaned := cleanup
			o.capture()
			if cleanup != cleaned || presents() != before {
				t.Fatal("capture rebuilt or presented the hidden cleanup frame")
			}
			s.occluded = false
			o.event(platform.SurfaceEvent{Kind: platform.SurfaceShown})
			o.frame()
			if presents() != before+1 || len(s.backgrounds) != backgrounds+1 {
				t.Fatal("shown window did not present fresh content and background")
			}
		})
	}
}

// An initially hidden visibility-aware window must draw before ready-to-show,
// so a ready callback can reveal it without waiting on a circular dependency.
func TestVisibilityReadAllowsFirstHiddenPresentation(t *testing.T) {
	o := newOccludedHost(t, true)
	o.rt = newRuntime(func(c *context) {
		if c.WindowHidden() {
			t.Error("first frame reported hidden before the window was ready")
		}
		coreText(c, "First picture")
	}, o.windowHost)
	o.capture()
	if o.ready != 0 {
		t.Fatal("capture made the window ready before presenting")
	}
	o.frame()
	if o.ready != 1 || o.s.pixels != 1 {
		t.Fatal("first hidden window frame did not present and become ready")
	}
}

// Views that do not opt into hidden cleanup retain their input-driven updates.
func TestOccludedInputWithoutVisibilityReadStillPresents(t *testing.T) {
	o := newOccludedHost(t, false)
	o.frame()
	before := o.s.pixels
	o.s.occluded = true
	o.count++
	o.event(platform.SurfaceEvent{Kind: platform.PointerDown, X: 10, Y: 10})
	o.frame()
	if o.s.pixels != before+1 {
		t.Fatal("ordinary occluded input frame was suppressed")
	}
}
