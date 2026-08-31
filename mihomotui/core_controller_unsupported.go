//go:build !linux

package mihomotui

func newSystemCoreController(string) coreController {
	return &processCoreController{process: NewMihomoProcess()}
}
