//go:build darwin && !ios

package e2e

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// A different application's full-screen Space is the important starting
// condition: Show keys our desktop window while AppKit is still bringing its
// Space into view. The peer belongs to this test and exits in cleanup.
func TestFullScreenAfterSpaceSwitch(t *testing.T) {
	for _, native := range []bool{true, false} {
		name := "webview"
		if native {
			name = "native"
		}
		t.Run(name, func(t *testing.T) {
			opts := mygo.WindowOptions{Hidden: true, Width: 480, Height: 320,
				TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: &mygo.Point{X: 20, Y: 20}}
			if native {
				opts.Content = ui.View(func(c *ui.Context) { ui.Text(c, "From another full-screen Space") })
			} else {
				opts.URL = "app://localhost/"
			}
			var w *mygo.Window
			ready := make(chan struct{}, 1)
			mygo.RunOnMain(func() {
				w = newWindow(t, opts)
				t.Cleanup(w.OnReadyToShow(func() { ready <- struct{}{} }))
			})
			wait.For(t, "target ready to show", ready)
			events := observeFullScreen(t, w)
			startFullScreenPeer(t)
			showForFullScreen(t, w)
			w.SetFullScreen(true)
			waitFullScreen(t, events, true)
			w.SetFullScreen(false)
			waitFullScreen(t, events, false)
		})
	}
}

func startFullScreenPeer(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Give the peer its own application identity. Two instances of this
	// test executable otherwise share AppKit's application/Space grouping.
	contents := filepath.Join(t.TempDir(), "FullScreenPeer.app", "Contents")
	if err := os.MkdirAll(filepath.Join(contents, "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.mygo.test.full-screen-peer</string>
<key>CFBundleExecutable</key><string>full-screen-peer</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>`), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	exe = filepath.Join(contents, "MacOS", "full-screen-peer")
	dst, err := os.OpenFile(exe, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy full-screen peer: %v, close: %v", copyErr, closeErr)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", filepath.Dir(contents)).CombinedOutput(); err != nil {
		t.Fatalf("ad-hoc sign full-screen test peer: %v\n%s", err, out)
	}
	r, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); writer.Close() })
	// t.Context is canceled before cleanups, when this peer must still
	// accept the close request and complete its native exit animation.
	ctx, cancel := context.WithTimeout(context.Background(), wait.Guard)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestFullScreenSpacePeer$", "-test.timeout=3m")
	cmd.Env = append(os.Environ(), "MYGO_E2E_FULLSCREEN_SPACE_PEER=1")
	cmd.ExtraFiles = []*os.File{writer}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		input.Close()
		t.Fatal(err)
	}
	writer.Close()
	ended := make(chan error, 1)
	go func() { ended <- cmd.Wait() }()
	t.Cleanup(func() {
		input.Write([]byte{1})
		input.Close()
		if err := wait.For(t, "full-screen peer exited", ended); err != nil {
			t.Errorf("full-screen peer: %v\n%s", err, output.String())
		}
	})
	ready := make(chan error, 1)
	go func() {
		var b [1]byte
		_, err := io.ReadFull(r, b[:])
		ready <- err
	}()
	if err := wait.For(t, "peer presented its full-screen Space", ready); err != nil {
		t.Fatalf("full-screen peer readiness: %v", err)
	}
}

func TestFullScreenSpacePeer(t *testing.T) {
	if os.Getenv("MYGO_E2E_FULLSCREEN_SPACE_PEER") != "1" {
		t.Skip("helper process for TestFullScreenAfterSpaceSwitch")
	}
	w := newWindow(t, mygo.WindowOptions{Width: 600, Height: 400,
		Content: ui.View(func(c *ui.Context) { ui.Text(c, "Full-screen Space peer") })})
	events := observeFullScreen(t, w)
	showForFullScreen(t, w)
	w.SetFullScreen(true)
	waitFullScreen(t, events, true)
	showForFullScreen(t, w)
	ready := os.NewFile(3, "full-screen-ready")
	if _, err := ready.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	ready.Close()
	var b [1]byte
	if _, err := io.ReadFull(os.Stdin, b[:]); err != nil {
		t.Fatal(err)
	}
	showForFullScreen(t, w)
	w.SetFullScreen(false)
	waitFullScreen(t, events, false)
}
