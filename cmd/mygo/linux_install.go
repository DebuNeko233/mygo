/*
Select Linux installation directories without changing package or executable identity.
*/

package main

import "strings"

func (c *Config) linuxInstallDirName() string {
	if name := strings.TrimSpace(c.Linux.InstallDirName); name != "" {
		return name
	}
	return c.slug()
}
