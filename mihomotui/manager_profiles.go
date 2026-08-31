package mihomotui

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type ProfileSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Active    bool   `json:"active"`
}

type ProfileImportRequest struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url"`
}

type ProfileRenameRequest struct {
	Name string `json:"name"`
}

func profileSummary(cfg *Config, profile SubscriptionMeta) ProfileSummary {
	active := cfg.ActiveSubscription >= 0 && cfg.ActiveSubscription < len(cfg.Subscriptions) && cfg.Subscriptions[cfg.ActiveSubscription].ID == profile.ID
	source := profileSourceLabel(profile.URL)
	if source == "[invalid-url]" && profile.SourceType != SubscriptionSourceURL {
		// Configurations created by older versions may be embedded/local and
		// legitimately have no URL. Keep truly malformed URL profiles visible
		// as errors while giving migrated local profiles an honest safe label.
		source = "本地迁移配置"
	}
	return ProfileSummary{ID: profile.ID, Name: profile.Name, Source: source, UpdatedAt: profile.UpdatedAt, Active: active}
}

// profileSourceLabel deliberately exposes only scheme and host. RedactURL is
// not sufficient here because subscription tokens are commonly placed in the
// path rather than the query string.
func profileSourceLabel(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "[invalid-url]"
	}
	return strings.ToLower(u.Scheme) + "://" + u.Host
}

func profileSummaries(cfg *Config) []ProfileSummary {
	items := make([]ProfileSummary, 0, len(cfg.Subscriptions))
	for _, profile := range cfg.Subscriptions {
		items = append(items, profileSummary(cfg, profile))
	}
	return items
}

// normalizeProfilePool keeps the legacy generator compatible while enforcing
// the new single-active-profile model. Subscription pools are no longer part
// of the public UI or API.
func normalizeProfilePool(cfg *Config) {
	members := make([]string, 0, len(cfg.Subscriptions))
	for _, item := range cfg.Subscriptions {
		members = append(members, item.ID)
	}
	activeID := ""
	if cfg.ActiveSubscription >= 0 && cfg.ActiveSubscription < len(cfg.Subscriptions) {
		activeID = cfg.Subscriptions[cfg.ActiveSubscription].ID
	}
	if len(members) == 0 {
		cfg.SubscriptionPools = nil
		cfg.ActiveSubscription = -1
		return
	}
	cfg.SubscriptionPools = []SubscriptionPool{{ID: "profiles", Name: "配置", Mode: SubscriptionPoolModeFailover, Members: members, ActiveMemberID: activeID, Enabled: true, RefreshInterval: DayInSeconds}}
}

func profilePoolNeedsNormalization(cfg *Config) bool {
	if len(cfg.Subscriptions) == 0 {
		return cfg.ActiveSubscription != -1 || len(cfg.SubscriptionPools) != 0
	}
	if len(cfg.SubscriptionPools) != 1 {
		return true
	}
	pool := cfg.SubscriptionPools[0]
	if pool.ID != "profiles" || pool.Mode != SubscriptionPoolModeFailover || !pool.Enabled || len(pool.Members) != len(cfg.Subscriptions) {
		return true
	}
	activeID := ""
	if cfg.ActiveSubscription >= 0 && cfg.ActiveSubscription < len(cfg.Subscriptions) {
		activeID = cfg.Subscriptions[cfg.ActiveSubscription].ID
	}
	if pool.ActiveMemberID != activeID {
		return true
	}
	for i := range cfg.Subscriptions {
		if pool.Members[i] != cfg.Subscriptions[i].ID {
			return true
		}
	}
	return false
}

func restoreConfigSnapshot(snapshot Config) error {
	_, err := UpdateGlobalConfig(func(current *Config) error {
		version := current.Version
		*current = snapshot.Clone()
		current.Version = version
		return nil
	})
	return err
}

func (d *Daemon) applyProfileConfig(previous Config) error {
	cfg := GlobalConfig()
	if err := cfg.GenerateMihomoConfig(); err != nil {
		_ = restoreConfigSnapshot(previous)
		old := GlobalConfig()
		_ = old.GenerateMihomoConfig()
		return fmt.Errorf("生成配置失败，已回滚: %w", err)
	}
	status, _ := d.managerStatus()
	if !status.Core.Running {
		return nil
	}
	if err := NewMihomoAPIFromConfig().ReloadConfigs(true); err != nil {
		_ = restoreConfigSnapshot(previous)
		old := GlobalConfig()
		_ = old.GenerateMihomoConfig()
		_ = NewMihomoAPIFromConfig().ReloadConfigs(true)
		return fmt.Errorf("内核运行配置应用失败，已回滚: %w", err)
	}
	return nil
}

func (d *Daemon) handleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := GlobalConfig()
		writeJSON(w, http.StatusOK, ok(profileSummaries(cfg)))
	case http.MethodPost:
		var req ProfileImportRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("请求无效: %w", err))
			return
		}
		if _, err := validateSubscriptionURL(req.URL); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		d.actionMu.Lock()
		defer d.actionMu.Unlock()
		before := GlobalConfig().Clone()
		if findSubscriptionByURL(&before, strings.TrimSpace(req.URL)) >= 0 {
			writeError(w, http.StatusConflict, fmt.Errorf("该配置链接已导入，请使用更新操作"))
			return
		}
		result, err := fetchSubscriptionWithOptions(req.URL, subscriptionFetchOptions{Strategy: SubscriptionFetchDirect})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		normalized, _, err := normalizeProfileContent(result.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id := newSubscriptionID()
		cache, digest, err := writeSubscriptionCache(id, normalized)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		name := uniqueSubscriptionName(req.Name, req.URL, &before)
		committed, err := UpdateGlobalConfig(func(cfg *Config) error {
			now := timestampNow()
			meta := SubscriptionMeta{ID: id, Name: name, URL: strings.TrimSpace(req.URL), SourceType: SubscriptionSourceURL, CacheFile: cache, ContentSHA256: digest, UpdatedAt: now, LastSuccessAt: now, LastCheckedAt: now}
			applySubscriptionFetchMetadata(&meta, result)
			cfg.Subscriptions = append(cfg.Subscriptions, meta)
			if cfg.ActiveSubscription < 0 {
				cfg.ActiveSubscription = len(cfg.Subscriptions) - 1
			}
			normalizeProfilePool(cfg)
			return nil
		})
		if err != nil {
			_ = os.Remove(cache)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if before.ActiveSubscription < 0 {
			if err := d.applyProfileConfig(before); err != nil {
				_ = os.Remove(cache)
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, http.StatusCreated, ok(profileSummary(&committed, committed.Subscriptions[len(committed.Subscriptions)-1])))
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("方法不允许"))
	}
}

func profileRoute(requestPath string) (id, action string, err error) {
	relative := strings.TrimPrefix(requestPath, "/v1/profiles/")
	parts := strings.Split(relative, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("配置 ID 不能为空")
	}
	id, err = url.PathUnescape(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("配置 ID 无效")
	}
	if len(parts) > 1 {
		action = parts[1]
	}
	if len(parts) > 2 {
		return "", "", fmt.Errorf("配置路径无效")
	}
	return id, action, nil
}

func (d *Daemon) handleProfileDetail(w http.ResponseWriter, r *http.Request) {
	id, action, err := profileRoute(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	before := GlobalConfig().Clone()
	idx := before.FindSubscriptionByIdentifier(id)
	if idx < 0 {
		writeError(w, http.StatusNotFound, fmt.Errorf("配置不存在"))
		return
	}
	profile := before.Subscriptions[idx]
	switch {
	case r.Method == http.MethodPost && action == "activate":
		if before.ActiveSubscription == idx {
			writeJSON(w, http.StatusOK, ok(profileSummary(&before, profile)))
			return
		}
		committed, err := UpdateGlobalConfig(func(cfg *Config) error {
			cfg.ActiveSubscription = cfg.FindSubscriptionByID(profile.ID)
			normalizeProfilePool(cfg)
			return nil
		})
		if err == nil {
			err = d.applyProfileConfig(before)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, ok(profileSummary(&committed, committed.Subscriptions[committed.ActiveSubscription])))
	case r.Method == http.MethodPost && action == "update":
		result, err := fetchSubscriptionWithOptions(profile.URL, subscriptionFetchOptions{Strategy: profile.FetchProxyStrategy, UserAgent: profile.UserAgent})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		normalized, _, err := normalizeProfileContent(result.Content)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		oldContent, _ := os.ReadFile(profile.CacheFile)
		cache, digest, err := writeSubscriptionCache(profile.ID, normalized)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		committed, err := UpdateGlobalConfig(func(cfg *Config) error {
			i := cfg.FindSubscriptionByID(profile.ID)
			if i < 0 {
				return fmt.Errorf("配置已被删除")
			}
			item := &cfg.Subscriptions[i]
			item.CacheFile, item.ContentSHA256, item.UpdatedAt, item.LastSuccessAt, item.LastCheckedAt = cache, digest, timestampNow(), timestampNow(), timestampNow()
			item.LastError = ""
			item.FailureCount = 0
			applySubscriptionFetchMetadata(item, result)
			return nil
		})
		if err == nil && before.ActiveSubscription == idx {
			err = d.applyProfileConfig(before)
		}
		if err != nil {
			if len(oldContent) > 0 {
				_, _, _ = writeSubscriptionCache(profile.ID, oldContent)
				// applyProfileConfig already restored the metadata snapshot, but it
				// had to regenerate/reload while the new cache bytes were still on
				// disk. Rebuild once more after restoring the old bytes so disk and
				// the running core are guaranteed to agree.
				restored := GlobalConfig()
				_ = restored.GenerateMihomoConfig()
				status, _ := d.managerStatus()
				if status.Core.Running {
					_ = NewMihomoAPIFromConfig().ReloadConfigs(true)
				}
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, ok(profileSummary(&committed, committed.Subscriptions[committed.FindSubscriptionByID(profile.ID)])))
	case r.Method == http.MethodPatch && action == "":
		var req ProfileRenameRequest
		if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("名称不能为空"))
			return
		}
		committed, err := UpdateGlobalConfig(func(cfg *Config) error {
			i := cfg.FindSubscriptionByID(profile.ID)
			if i < 0 {
				return fmt.Errorf("配置已被删除")
			}
			if other := cfg.FindSubscriptionByName(strings.TrimSpace(req.Name)); other >= 0 && other != i {
				return fmt.Errorf("名称已存在")
			}
			cfg.Subscriptions[i].Name = strings.TrimSpace(req.Name)
			return nil
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, ok(profileSummary(&committed, committed.Subscriptions[committed.FindSubscriptionByID(profile.ID)])))
	case r.Method == http.MethodDelete && action == "":
		status, _ := d.managerStatus()
		if len(before.Subscriptions) == 1 && before.ActiveSubscription == idx && status.Core.Running {
			writeError(w, http.StatusConflict, fmt.Errorf("请先停止代理内核，再删除最后一个活动配置"))
			return
		}
		wasActive := before.ActiveSubscription == idx
		committed, err := UpdateGlobalConfig(func(cfg *Config) error {
			if err := cfg.RemoveSubscription(profile.Name); err != nil {
				return err
			}
			normalizeProfilePool(cfg)
			return nil
		})
		if err == nil && wasActive && len(committed.Subscriptions) > 0 {
			err = d.applyProfileConfig(before)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		_ = os.Remove(profile.CacheFile)
		writeJSON(w, http.StatusOK, ok(profileSummaries(&committed)))
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("操作或方法不支持"))
	}
}
