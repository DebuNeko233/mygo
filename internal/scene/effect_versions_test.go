package scene

import "testing"

// Stable shader images do not damage their output, but new pixels, images and
// borrowed uniform changes do, including after the original words are reused.
func TestEffectImageDamageVersions(t *testing.T) {
	for _, mutation := range []string{"pixels", "replacement", "uniform", "wide uniform", "canvas"} {
		t.Run(mutation, func(t *testing.T) {
			img := NewImageRGBA(2, 2, make([]byte, 16))
			if mutation == "canvas" {
				img = NewCanvas(2, 2)
			}
			words, wide := []uint32{17}, []uint32{23}
			fx := &Effect{}
			build := func() *Scene {
				return &Scene{Width: 100, Height: 100,
					Ops:     []Op{{Kind: OpEffect, Rect: Rect{X: 10, Y: 10, W: 20, H: 20}}},
					Effects: []EffectOp{{Effect: fx, Images: [4]*Image{img}, Uniforms: words, WideUniforms: wide}}}
			}
			d := Differ{}
			d.Diff(build())
			d.Remember(build())
			if damage, ok := d.Diff(build()); !ok || len(damage) != 0 {
				t.Fatalf("stable input: damage %v, known %v", damage, ok)
			}
			switch mutation {
			case "pixels", "canvas":
				img.Changed()
			case "replacement":
				img = NewImageRGBA(2, 2, make([]byte, 16))
			case "uniform":
				words[0]++
			case "wide uniform":
				wide[0]++
			}
			if damage, ok := d.Diff(build()); !ok || len(damage) == 0 {
				t.Fatalf("changed %s was skipped", mutation)
			}
		})
	}
}

type changingEffectTexture struct{}

func (changingEffectTexture) GPUTexture(GPUAPI, uintptr) uintptr { return 0 }

// External textures and video may advance without a scene image-version update.
func TestEffectExternalImagesStillDamage(t *testing.T) {
	for _, video := range []bool{false, true} {
		img := NewImageRGBA(2, 2, make([]byte, 16))
		if video {
			img.Surface = &Surface{}
		} else {
			img.GPU = changingEffectTexture{}
		}
		s := &Scene{Width: 100, Height: 100, Ops: []Op{{Kind: OpEffect, Rect: Rect{W: 20, H: 20}}},
			Effects: []EffectOp{{Effect: &Effect{}, Images: [4]*Image{img}}}}
		d := Differ{}
		d.Diff(s)
		d.Remember(s)
		if damage, _ := d.Diff(s); len(damage) == 0 {
			t.Fatal("external image was treated as immutable")
		}
	}
}
