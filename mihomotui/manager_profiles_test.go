package mihomotui

import "testing"

func TestProfileSummarySourceLabels(t *testing.T) {
	cfg := &Config{ActiveSubscription: 0}

	remote := profileSummary(cfg, SubscriptionMeta{ID: "remote", URL: "https://example.com/private/token?key=value", SourceType: SubscriptionSourceURL})
	if remote.Source != "https://example.com" {
		t.Fatalf("remote source leaked or was mislabeled: %q", remote.Source)
	}

	local := profileSummary(cfg, SubscriptionMeta{ID: "local", SourceType: SubscriptionSourceContent})
	if local.Source != "本地迁移配置" {
		t.Fatalf("local migration source = %q", local.Source)
	}

	broken := profileSummary(cfg, SubscriptionMeta{ID: "broken", URL: "://bad", SourceType: SubscriptionSourceURL})
	if broken.Source != "[invalid-url]" {
		t.Fatalf("broken URL source = %q", broken.Source)
	}
}
