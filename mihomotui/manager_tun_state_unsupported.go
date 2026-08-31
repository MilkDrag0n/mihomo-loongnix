//go:build !linux

package mihomotui

func tunCleanupNeeded(status TUNRuntimeStatus) bool {
	return status.Configured || status.RuntimeEnabled || status.InterfacePresent
}
