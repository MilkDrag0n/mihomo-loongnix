package mihomotui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type coreServiceState struct {
	Active bool
	PID    int
	Detail string
}

type coreController interface {
	Start(context.Context) error
	Stop(context.Context) error
	Restart(context.Context) error
	State(context.Context) (coreServiceState, error)
}

type processCoreController struct{ process *MihomoProcess }

func (c *processCoreController) Start(context.Context) error   { return c.process.Start() }
func (c *processCoreController) Stop(context.Context) error    { return c.process.Stop() }
func (c *processCoreController) Restart(context.Context) error { return c.process.Restart() }
func (c *processCoreController) State(context.Context) (coreServiceState, error) {
	running, pid := c.process.Status()
	return coreServiceState{Active: running, PID: pid}, nil
}

func (d *Daemon) ensureCoreController() coreController {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.core != nil {
		return d.core
	}
	if d.mihomoProcess == nil {
		d.mihomoProcess = NewMihomoProcess()
	}
	if os.Getenv("MIHOMO_TUI_CORE_SERVICE") != "" {
		d.core = newSystemCoreController(os.Getenv("MIHOMO_TUI_CORE_SERVICE"))
	} else {
		d.core = &processCoreController{process: d.mihomoProcess}
	}
	return d.core
}

func parseSystemdShow(output string) coreServiceState {
	state := coreServiceState{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "ActiveState":
			state.Active = value == "active"
			state.Detail = value
		case "MainPID":
			state.PID, _ = strconv.Atoi(value)
		}
	}
	return state
}

func waitForCore(ctx context.Context, wantRunning bool, status func() (ManagerStatus, error)) (ManagerStatus, error) {
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	var last ManagerStatus
	for {
		current, err := status()
		if err == nil {
			last = current
			if current.Core.Running == wantRunning {
				return current, nil
			}
		}
		select {
		case <-ctx.Done():
			return last, fmt.Errorf("等待内核状态超时: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
