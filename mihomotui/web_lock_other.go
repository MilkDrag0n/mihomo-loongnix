//go:build !linux

package mihomotui

import "fmt"

func lockWebDeployment() (func(), error) { return nil, fmt.Errorf("Web 服务控制仅支持 Linux") }
