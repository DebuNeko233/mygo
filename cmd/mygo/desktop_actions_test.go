/*
Verify launcher actions preserve argument boundaries and cannot inject desktop entry groups.
*/

package main

import (
	"strings"
	"testing"
)

// Actions invoke the same executable as the main entry without forwarding
// file placeholders; arguments retain empty values, spaces and literal %.
func TestLinuxDesktopActions(t *testing.T) {
	c := &Config{Name: "Player", Slug: "player", URLSchemes: []string{"player"}, Linux: Linux{Actions: []LinuxDesktopAction{{ID: "PlayPause", Name: "Play/Pause", Args: []string{"--play-pause", "", "two words", "50%", "a;b"}, Icon: "media-playback-start"}}}}
	entry := linuxDesktopEntry(c, "/opt/My Player/player", "player")
	if !strings.Contains(entry, "Exec=\"/opt/My Player/player\" %U\n") || !strings.Contains(entry, "Actions=PlayPause;\n") {
		t.Fatal("adding actions lost the main entry's URL handling")
	}
	_, action, found := strings.Cut(entry, "[Desktop Action PlayPause]\n")
	if !found || !strings.Contains(action, `Exec="/opt/My Player/player" --play-pause "" "two words" "50%%" "a;b"`+"\n") || strings.Contains(action, "%U") {
		t.Fatalf("action command lost argument boundaries: %s", action)
	}
}

// Reject duplicate/invalid action IDs and control characters before any
// desktop file is written, rather than injecting keys or action groups.
func TestLinuxDesktopActionsValidation(t *testing.T) {
	valid := LinuxDesktopAction{ID: "Play-Pause", Name: "Play/Pause", Args: []string{"--toggle"}}
	for _, actions := range [][]LinuxDesktopAction{
		{{ID: "", Name: "Empty ID"}},
		{{ID: "Bad;ID", Name: "Bad ID"}},
		{{ID: "A", Name: ""}},
		{valid, valid},
		{{ID: "A", Name: "Injected\nExec=other"}},
		{{ID: "A", Name: "A", Icon: "icon\n[Desktop Entry]"}},
		{{ID: "A", Name: "A", Args: []string{"one\nTwo"}}},
	} {
		c := &Config{Name: "Player", Linux: Linux{Actions: actions}}
		c.applyDefaults()
		if err := c.validate(); err == nil || !strings.Contains(err.Error(), "linux.actions") {
			t.Errorf("invalid desktop actions accepted: %+v (%v)", actions, err)
		}
	}
	c := &Config{Name: "Player", Linux: Linux{Actions: []LinuxDesktopAction{valid}}}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}
