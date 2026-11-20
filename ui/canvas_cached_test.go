package ui

import (
	"image"
	"image/color"
	"testing"
)

// Repeated cached paints retain pixels while image updates and an intervening
// ordinary draw replace the visible content immediately.
func TestCanvasDrawCachedUpdates(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+3] = 255, 255
	}
	bm, cv := NewBitmap(img), NewCanvas(4, 4)
	ordinary := false
	tt := coreNewTester(func(c *context) {
		coreBox(c).Size(4, 4).Draw(func(p *Painter, r Rect) {
			if ordinary {
				cv.Draw(p, func(cp *Painter) { cp.Fill(Rect{W: 4, H: 4}, RGB(0, 255, 0), 0) })
			} else {
				cv.DrawCached(p, func(cp *Painter) { cp.Image(bm, Rect{W: 4, H: 4}, FillBox) })
			}
			p.Image(cv.Bitmap(), r, FillBox)
		})
	}, 4, 4)
	version := cv.img.Version()
	tt.Frame()
	if cv.img.Version() != version {
		t.Fatal("unchanged canvas was rendered anew")
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = 0, 255
	}
	bm.Update(img)
	tt.Frame()
	if got := tt.Image().RGBAAt(2, 2); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("image update: %v", got)
	}
	ordinary = true
	tt.Frame()
	if got := tt.Image().RGBAAt(2, 2); got != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("ordinary draw: %v", got)
	}
	ordinary = false
	tt.Frame()
	if got := tt.Image().RGBAAt(2, 2); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("cached draw after ordinary draw: %v", got)
	}
}

// A cached effect must keep the same input resolution and inset as an uncached
// effect, even when the element is transformed and the window changes scale.
func TestCachedEffectKeepsPixelScale(t *testing.T) {
	for _, scale := range []float32{0.5, 1, 2} {
		cached := false
		tt := coreNewTester(func(c *context) {
			coreBox(c).Fill().Background(RGB(255, 255, 255)).Padding(12).Children(func() {
				e := coreBox(c).Size(30, 20).TransformOrigin(0, 0).Scale(scale, scale).Effect(imageEffect{region: true}).Draw(func(p *Painter, r Rect) {
					p.Fill(Rect{r.X, r.Y, r.W / 2, r.H}, RGB(220, 30, 60), 0)
					p.Fill(Rect{r.X + r.W/2, r.Y, r.W / 2, r.H}, RGB(10, 140, 220), 0)
				})
				if cached {
					e.Cache()
				}
			})
		}, 100, 100)
		for _, windowScale := range []float32{1, 2} {
			cached = false
			tt.SetScale(windowScale)
			tt.Frame()
			want := tt.Image()
			cached = true
			tt.Frame()
			if diff := sameImage(want, tt.Image()); diff != "" {
				t.Fatalf("transform %v, window scale %v: %s", scale, windowScale, diff)
			}
		}
	}
}
