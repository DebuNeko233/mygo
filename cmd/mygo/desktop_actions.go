/*
Validate desktop action identifiers and escape string values for generated Linux launcher entries.
*/

package main

import (
	"regexp"
	"strings"
)

var desktopActionIDRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func desktopActionValue(value string) string {
	return strings.ReplaceAll(value, `\`, `\\`)
}
