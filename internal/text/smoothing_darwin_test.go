//go:build darwin && !ios

/*
Checks Core Graphics bitmap glyph coverage across smoothing preferences and
display scales, including explicit thick/flat text and color emoji.
*/

package text

import (
	"bytes"
	"fmt"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

// Bitmap contexts must allow smoothing as well as request it. A context that
// only requests it silently draws Thick, Flat and every shade identically.
func TestDarwinGlyphSmoothing(t *testing.T) {
	s := newSystem()
	if err := s.RegisterFont(gomono.TTF, "Smoothing Test Mono"); err != nil {
		t.Fatal(err)
	}
	for _, family := range []string{"", "Smoothing Test Mono"} {
		for _, scale := range []float32{1, 1.5, 2} {
			for _, smoothing := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/scale=%g/smoothing=%v", family, scale, smoothing), func(t *testing.T) {
					l := s.Layout(Params{Text: "O", Style: Style{Family: family, Size: 13}})
					g := l.Lines[0].Glyphs[0]
					// Model the preference on this font without changing NSUserDefaults.
					f := *g.Font
					f.shaded = smoothing
					ink := func(shade Shade) (n int) {
						img := s.Glyph(&f, g.ID, scale, 0, shade, false)
						if !img.OK || img.Colored {
							t.Fatal("missing monochrome glyph")
						}
						for y := range int(img.H) {
							for x := range int(img.W) {
								n += int(s.MaskAtlas.Pix[(int(img.Y)+y)*s.MaskAtlas.W+int(img.X)+x])
							}
						}
						return n
					}
					flat, dark, light, thick := ink(Flat), ink(0), ink(Shades-1), ink(Thick)
					if thick <= dark || thick < light {
						t.Fatalf("explicit thick text did not thicken: flat=%d dark=%d light=%d thick=%d", flat, dark, light, thick)
					}
					if smoothing {
						if dark <= flat || light <= dark {
							t.Fatalf("smoothing did not follow the text's shade: flat=%d dark=%d light=%d", flat, dark, light)
						}
					} else if dark != flat || light != flat {
						t.Fatalf("disabled smoothing changed coverage: flat=%d dark=%d light=%d", flat, dark, light)
					}
				})
			}
		}
	}
}

// Requesting thicker text must preserve a color glyph's RGBA pixels.
func TestDarwinThickPreservesColorGlyphs(t *testing.T) {
	s := newSystem()
	l := s.Layout(Params{Text: "🎉", Style: Style{Size: 24}})
	g := l.Lines[0].Glyphs[0]
	pixels := func(shade Shade) []byte {
		img := s.Glyph(g.Font, g.ID, 2, 0, shade, false)
		if !img.OK || !img.Colored {
			t.Fatal("missing color emoji")
		}
		pix := make([]byte, 0, 4*int(img.W)*int(img.H))
		for y := range int(img.H) {
			i := 4 * ((int(img.Y)+y)*s.ColorAtlas.W + int(img.X))
			pix = append(pix, s.ColorAtlas.Pix[i:i+4*int(img.W)]...)
		}
		return pix
	}
	if !bytes.Equal(pixels(0), pixels(Thick)) {
		t.Fatal("thick text changed color emoji")
	}
}
