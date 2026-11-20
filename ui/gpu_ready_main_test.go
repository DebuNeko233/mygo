package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

type mainReadyGPU struct{ waitingGPU }

func (*mainReadyGPU) ReadyOnMainThread() bool { return true }

// A renderer completing on the UI thread draws the waiting frame without
// another post, but defers a completion that would reenter a frame.
func TestReadyOnMainAvoidsRepost(t *testing.T) {
	g := &mainReadyGPU{waitingGPU: waitingGPU{ready: true}}
	h, _, frame := gpuHost(t, func() (gpuRenderer, error) { return g, nil })
	t.Cleanup(h.detach)
	s := &pacedSurface{h: h}
	h.conn.Surface = s
	var posted []func()
	h.conn.Post = func(fn func()) { posted = append(posted, fn) }
	frame()
	for _, guard := range []string{"none", "framing", "building"} {
		before := g.frames
		g.ready = false
		h.event(platform.SurfaceEvent{Kind: platform.SurfaceFrame, Paced: true})
		if !h.waiting || g.wake == nil {
			t.Fatal("did not wait")
		}
		g.ready = true
		h.framing = guard == "framing"
		h.rt.inFrame = guard == "building"
		g.wake()
		h.framing, h.rt.inFrame = false, false
		if guard == "none" {
			if len(posted) != 0 || g.frames != before+1 {
				t.Fatalf("main-thread completion: frames=%d posted=%d", g.frames-before, len(posted))
			}
		} else {
			if len(posted) != 1 || g.frames != before {
				t.Fatalf("%s completion reentered a frame", guard)
			}
			fn := posted[0]
			posted = nil
			fn()
			if g.frames != before+1 {
				t.Fatal("deferred completion did not draw")
			}
		}
	}
}
