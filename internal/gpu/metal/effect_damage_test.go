//go:build darwin && !ios

package metal

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/wait"
)

// Image-input shaders drawn in part must match whole-frame pixels through
// image updates, uniform changes and triple-buffer reuse, in both color formats.
func TestPartialShaderImageUpdates(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprint("wide=", wide), func(t *testing.T) {
			r, err := newRenderer()
			if err != nil {
				unavailable(t, err)
			}
			defer r.Release()
			pix := []byte{200, 100, 50, 255}
			img := scene.NewImageRGBA(1, 1, pix)
			words := []uint32{math.Float32bits(1), math.Float32bits(1), math.Float32bits(1), math.Float32bits(1)}
			fx := &scene.Effect{Name: "image damage", Shader: &scene.ShaderLayout{Lanes: []int32{0, 4, 8, 12}, Size: 16}, Metal: scene.EffectCode{Source: ShaderSource(`
struct Uniforms { float4 tint; };
float4 shaderMain(constant Uniforms& uniforms, texture2d<float> T0, texture2d<float> T1, texture2d<float> T2, texture2d<float> T3, float2 p, float4 rect, thread const Inst& i) {
 return T0.read(uint2(0)) * uniforms.tint;
}`)}}
			s := &scene.Scene{Width: 240, Height: 180, Clear: scene.Color{R: 20, G: 30, B: 40, A: 255},
				Ops:     []scene.Op{{Kind: scene.OpEffect, Rect: scene.Rect{W: 240, H: 180}}, {Kind: scene.OpEffect, Start: 1, Rect: scene.Rect{X: 10, Y: 10, W: 50, H: 40}}},
				Effects: []scene.EffectOp{{Effect: fx, Uniforms: append([]uint32(nil), words...), Images: [4]*scene.Image{scene.NewImageRGBA(1, 1, []byte{64, 25, 50, 128})}}, {Effect: fx, Uniforms: words, Images: [4]*scene.Image{img}}}}
			n := partialFrames(t, r, s, wide, 24, func(step int) string {
				if step%8 == 0 {
					s.Clear.R += 17
				}
				if step%2 == 0 {
					pix[0] = byte(200 - step*5)
					img.Changed()
				} else {
					words[1] = math.Float32bits(1 - float32(step)/30)
				}
				return fmt.Sprint("image/uniform ", step)
			})
			if n == 0 {
				t.Fatal("no frame used partial rendering")
			}
		})
	}
}

// Frames waiting for drawables retain small damage through image-independent
// effects; a framebuffer-reading effect still draws its entire backdrop.
func TestPendingEffectFramesRedrawOnlyDamage(t *testing.T) {
	for _, backdrop := range []bool{false, true} {
		t.Run(fmt.Sprint("backdrop=", backdrop), func(t *testing.T) {
			if err := load(); err != nil {
				t.Skip("no Metal:", err)
			}
			var layer id
			pool(func() { layer = send(class("CALayer"), "new") })
			defer pool(func() { release(&layer) })
			r, err := New(uintptr(layer))
			if err != nil {
				unavailable(t, err)
			}
			defer r.Release()
			posted := make(chan func(), 4)
			r.SetPost(func(fn func()) { posted <- fn })
			gate := make(chan struct{})
			next := nextDrawable
			nextDrawable = func(l id) id { <-gate; return next(l) }
			defer func() {
				close(gate)
				r.pre.wait() // finish the fetch before restoring its shared test hook
				nextDrawable = next
			}()
			fx := gputest.TintEffect
			if backdrop {
				fx = gputest.LensEffect
			}
			s := &scene.Scene{Width: 160, Height: 100, Clear: scene.Color{A: 255},
				Ops:     []scene.Op{{Kind: scene.OpFill, Rect: scene.Rect{X: 10, Y: 10, W: 8, H: 8}, Color: scene.Color{R: 200, A: 255}}, {Kind: scene.OpEffect, Rect: scene.Rect{W: 160, H: 100}}},
				Effects: []scene.EffectOp{{Effect: fx, Params: [5][4]float32{{0.2}, {0.2, 0.4, 0.6, 0.4}}}}}
			partial := 0
			for step := range 12 {
				s.Ops[0].Color.R = byte(100 + step*10)
				if err := r.Render(s); err != nil {
					t.Fatal(err)
				}
				if step >= 2 {
					if r.Ready(nil) {
						t.Fatal("frame did not wait for a drawable")
					}
					gate <- struct{}{}
					finish := wait.For(t, "pending effect frame", posted)
					finish()
				}
				r.waitLast()
				if r.redrew == redrewPart {
					partial++
				}
			}
			if backdrop && partial != 0 {
				t.Fatalf("framebuffer-reading effect drew %d partial frames", partial)
			}
			if !backdrop && partial == 0 {
				t.Fatal("small updates under an effect always redrew the whole frame")
			}
		})
	}
}
