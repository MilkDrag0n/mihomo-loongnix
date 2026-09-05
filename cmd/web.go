package cmd

import (
	"encoding/json"
	"fmt"
	"mihomotui/mihomotui"
	"os"
)

func RunWeb(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法：mihomo-tui web status|start|stop")
	}
	c, e := mihomotui.NewIPCClient()
	if e != nil {
		return e
	}
	s, e := c.ManagerWeb(args[0])
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(s)
}
