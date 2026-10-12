//go:build linux || darwin

package main

import "github.com/spf13/cobra"

func addPlatformMountFlags(*cobra.Command, *mountOptions) {}
func platformMountCommands() []*cobra.Command             { return nil }
func runPlatformMount(cmd *cobra.Command, credentials authFlags, opts mountOptions) error {
	return runMount(cmd, credentials, opts)
}
