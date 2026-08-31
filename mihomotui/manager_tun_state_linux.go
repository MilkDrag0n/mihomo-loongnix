//go:build linux

package mihomotui

import "os"

func tunCleanupNeeded(status TUNRuntimeStatus) bool {
	if status.Configured || status.RuntimeEnabled || status.InterfacePresent {
		return true
	}
	_, err := os.Stat(tunRoutingStatePath())
	return err == nil
}
