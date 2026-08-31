//go:build linux

package mihomotui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type systemCoreController struct{ unit string }

func newSystemCoreController(unit string) coreController {
	if strings.TrimSpace(unit) == "" {
		unit = "mihomo.service"
	}
	return &systemCoreController{unit: unit}
}

func (c *systemCoreController) command(ctx context.Context, verb string) error {
	output, err := exec.CommandContext(ctx, "systemctl", verb, c.unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s %s 失败: %w: %s", verb, c.unit, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (c *systemCoreController) Start(ctx context.Context) error   { return c.command(ctx, "start") }
func (c *systemCoreController) Stop(ctx context.Context) error    { return c.command(ctx, "stop") }
func (c *systemCoreController) Restart(ctx context.Context) error { return c.command(ctx, "restart") }
func (c *systemCoreController) State(ctx context.Context) (coreServiceState, error) {
	output, err := exec.CommandContext(ctx, "systemctl", "show", c.unit, "--property=ActiveState", "--property=MainPID").CombinedOutput()
	if err != nil {
		return coreServiceState{}, fmt.Errorf("读取 %s 状态失败: %w: %s", c.unit, err, strings.TrimSpace(string(output)))
	}
	return parseSystemdShow(string(output)), nil
}
