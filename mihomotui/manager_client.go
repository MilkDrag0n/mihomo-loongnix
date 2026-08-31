package mihomotui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func (c *IPCClient) ManagerStatus() (*ManagerStatus, error) {
	resp, err := c.requestJSON(http.MethodGet, "/v1/status", nil, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ManagerStatus](resp)
	return &result, err
}

func (c *IPCClient) ManagerStartCore() (*ManagerStatus, error) { return c.managerCoreAction("start") }
func (c *IPCClient) ManagerStopCore() (*ManagerStatus, error)  { return c.managerCoreAction("stop") }
func (c *IPCClient) managerCoreAction(action string) (*ManagerStatus, error) {
	resp, err := c.requestJSON(http.MethodPost, "/v1/core/"+action, nil, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ManagerStatus](resp)
	return &result, err
}

func (c *IPCClient) ManagerSetTUN(enabled bool) (*ManagerStatus, error) {
	body, _ := json.Marshal(TUNSetRequest{Enabled: enabled})
	resp, err := c.requestJSON(http.MethodPut, "/v1/tun", body, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ManagerStatus](resp)
	return &result, err
}

func (c *IPCClient) ManagerSetProxyPort(port int) (*ManagerStatus, error) {
	body, _ := json.Marshal(ProxyPortSetRequest{Port: port})
	resp, err := c.requestJSON(http.MethodPut, "/v1/proxy-port", body, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ManagerStatus](resp)
	return &result, err
}

func (c *IPCClient) ManagerProfiles() ([]ProfileSummary, error) {
	resp, err := c.requestJSON(http.MethodGet, "/v1/profiles", nil, nil)
	if err != nil {
		return nil, err
	}
	return unmarshalData[[]ProfileSummary](resp)
}

func (c *IPCClient) ManagerImportProfile(name, rawURL string) (*ProfileSummary, error) {
	body, _ := json.Marshal(ProfileImportRequest{Name: name, URL: rawURL})
	resp, err := c.requestJSON(http.MethodPost, "/v1/profiles", body, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ProfileSummary](resp)
	return &result, err
}

func (c *IPCClient) ManagerActivateProfile(id string) (*ProfileSummary, error) {
	return c.managerProfileAction(id, "activate")
}
func (c *IPCClient) ManagerUpdateProfile(id string) (*ProfileSummary, error) {
	return c.managerProfileAction(id, "update")
}
func (c *IPCClient) managerProfileAction(id, action string) (*ProfileSummary, error) {
	resp, err := c.requestJSON(http.MethodPost, "/v1/profiles/"+url.PathEscape(id)+"/"+action, nil, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ProfileSummary](resp)
	return &result, err
}

func (c *IPCClient) ManagerRenameProfile(id, name string) (*ProfileSummary, error) {
	body, _ := json.Marshal(ProfileRenameRequest{Name: name})
	resp, err := c.requestJSON(http.MethodPatch, "/v1/profiles/"+url.PathEscape(id), body, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ProfileSummary](resp)
	return &result, err
}

func (c *IPCClient) ManagerDeleteProfile(id string) ([]ProfileSummary, error) {
	resp, err := c.requestJSON(http.MethodDelete, "/v1/profiles/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return nil, err
	}
	return unmarshalData[[]ProfileSummary](resp)
}

func (c *IPCClient) ManagerProxyGroups() ([]ProxyGroup, error) {
	resp, err := c.requestJSON(http.MethodGet, "/v1/proxy-groups", nil, nil)
	if err != nil {
		return nil, err
	}
	return unmarshalData[[]ProxyGroup](resp)
}

func (c *IPCClient) ManagerSelectProxy(group, node string) (*ProxyGroup, error) {
	body, _ := json.Marshal(ProxySelectRequest{Name: node})
	resp, err := c.requestJSON(http.MethodPut, "/v1/proxy-groups/"+url.PathEscape(group), body, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ProxyGroup](resp)
	return &result, err
}

func (c *IPCClient) ManagerTestProxyDelay(group, node string) (*ProxyDelayResponse, error) {
	body, _ := json.Marshal(ProxyDelayTestRequest{Group: group, Name: node})
	resp, err := c.requestJSON(http.MethodPost, "/v1/proxy-delay", body, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[ProxyDelayResponse](resp)
	return &result, err
}

func (c *IPCClient) ManagerRules() ([]Rule, error) {
	resp, err := c.requestJSON(http.MethodGet, "/v1/rules", nil, nil)
	if err != nil {
		return nil, err
	}
	return unmarshalData[[]Rule](resp)
}

func (c *IPCClient) ManagerLoggingStatus() (*LoggingStatus, error) {
	resp, err := c.requestJSON(http.MethodGet, "/v1/logging/status", nil, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[LoggingStatus](resp)
	return &result, err
}

func (c *IPCClient) ManagerSetLogging(enabled bool) (*LoggingStatus, error) {
	body, _ := json.Marshal(LoggingSetRequest{Enabled: enabled})
	resp, err := c.requestJSON(http.MethodPut, "/v1/logging", body, nil)
	if err != nil {
		return nil, err
	}
	result, err := unmarshalData[LoggingStatus](resp)
	return &result, err
}

func (c *IPCClient) ManagerLogStream(level string) (*http.Response, error) {
	query := map[string]string{}
	if level != "" {
		query["level"] = level
	}
	resp, err := c.streamRequest(http.MethodGet, "/v1/logs/stream", query)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("Manager 日志流返回 %s", resp.Status)
	}
	return resp, nil
}
