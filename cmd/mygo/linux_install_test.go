/*
Verify Linux installation folders stay independent of package/update identity.
*/
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Invalid components cannot redirect an installer outside its application folder.
func TestLinuxInstallDirValidation(t *testing.T) {
	for _, name := range []string{"..", "/tmp/app", "a/b", "a\\b", " ", "a\napp", "$app", "a%u", "a`b", "a\"b"} {
		c := &Config{Name: "Nyago Music", Slug: "nyago-music", Linux: Linux{InstallDirName: name}}
		c.applyDefaults()
		if err := c.validate(); err == nil || !strings.Contains(err.Error(), "linux.installDirName") {
			t.Errorf("accepted invalid folder %q: %v", name, err)
		}
	}
	c := &Config{Name: "Nyago Music", Slug: "nyago-music", Linux: Linux{InstallDirName: "Nyago Music"}}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}

// Real DEB/RPM payloads and desktop commands use a spaced folder without renaming their package.
func TestLinuxInstallDirPackages(t *testing.T) {
	c := &Config{Name: "Nyago Music", Slug: "nyago-music", Version: "1.0.0", Linux: Linux{InstallDirName: "Nyago Music", Command: commandNames{"nyago-music"}, WebView: new(false)}}
	c.Linux.Actions = []LinuxDesktopAction{{ID: "PlayPause", Name: "Play/Pause", Args: []string{"--play-pause"}}}
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "nyago-music"), []byte("#!/bin/sh\necho fixture\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeLinuxDesktop(c, stage, "nyago-music"); err != nil {
		t.Fatal(err)
	}
	app, err := entryNames(stage)
	if err != nil {
		t.Fatal(err)
	}
	deb, err := writeDeb(c, stage, "nyago-music", "amd64", app)
	if err != nil {
		t.Fatal(err)
	}
	files := debData(t, deb)
	if _, ok := files["opt/Nyago Music/nyago-music"]; !ok {
		t.Fatal("DEB payload has no configured app path")
	}
	if got := files["usr/bin/nyago-music"]; got != "../../opt/Nyago Music/nyago-music" {
		t.Fatalf("bad command link: %q", got)
	}
	if got := files["usr/share/applications/nyago-music.desktop"]; !strings.Contains(got, "Exec=\"/opt/Nyago Music/nyago-music\"") {
		t.Fatalf("unquoted desktop path: %s", got)
	}
	if got := files["usr/share/applications/nyago-music.desktop"]; !strings.Contains(got, "Actions=PlayPause;\n") || !strings.Contains(got, "Exec=\"/opt/Nyago Music/nyago-music\" --play-pause\n") {
		t.Fatalf("DEB action does not run the installed app: %s", got)
	}
	if got := debMember(t, deb, "control.tar.gz")["control"]; !strings.HasPrefix(got, "Package: nyago-music\n") {
		t.Fatal("directory override renamed the package")
	}
	rpm, err := writeRpm(c, stage, "nyago-music", "amd64", app)
	if err != nil {
		t.Fatal(err)
	}
	if tar, err := exec.LookPath("bsdtar"); err == nil {
		out, err := exec.Command(tar, "-tf", rpm).CombinedOutput()
		if err != nil {
			t.Fatalf("reading RPM payload: %v %s", err, out)
		}
		if !strings.Contains(string(out), "opt/Nyago Music/nyago-music") {
			t.Fatalf("RPM payload has wrong folder: %s", out)
		}
	}
}
