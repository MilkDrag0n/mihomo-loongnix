package mihomotui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagedLoggingDefaultIsDisabled(t *testing.T) {
	cfg := defaultConfig()
	if cfg.ManagedLogging.Enabled {
		t.Fatal("managed disk logging must default to disabled")
	}
	if cfg.ManagedLogging.MaxFileBytes != 10<<20 || cfg.ManagedLogging.MaxBackups != 3 {
		t.Fatalf("unexpected defaults: %+v", cfg.ManagedLogging)
	}
}

func TestManagedLogRecorderDisabledDoesNotSubscribeOrCreateFile(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	recorder := NewManagedLogRecorder(dir, func() (*MihomoAPI, error) {
		calls.Add(1)
		return nil, fmt.Errorf("must not be called")
	})
	defer recorder.Close()
	if err := recorder.Apply(ManagedLoggingConfig{Enabled: false, MaxFileBytes: 10 << 20, MaxBackups: 3}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("disabled recorder subscribed %d times", calls.Load())
	}
	if _, err := os.Stat(dir + "/" + managedLogFileName); !os.IsNotExist(err) {
		t.Fatalf("disabled recorder created a file: %v", err)
	}
}

func TestManagedLogRecorderToggleSizeAndRotation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 120; i++ {
			prefix := "data: "
			if i%2 == 0 {
				prefix = ""
			}
			fmt.Fprintf(w, "%s{\"type\":\"info\",\"payload\":\"%s-%03d\"}\n\n", prefix, strings.Repeat("x", 48), i)
			if flusher != nil {
				flusher.Flush()
			}
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	dir := t.TempDir()
	recorder := NewManagedLogRecorder(dir, func() (*MihomoAPI, error) { return NewMihomoAPI(server.URL, ""), nil })
	defer recorder.Close()
	if status := recorder.Status(); status.Enabled || status.TotalBytes != 0 {
		t.Fatalf("initial status=%+v", status)
	}
	if err := recorder.Apply(ManagedLoggingConfig{Enabled: true, MaxFileBytes: 512, MaxBackups: 3}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(dir + "/" + managedLogFileName + ".3"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	status := recorder.Status()
	if !status.Enabled || status.TotalBytes == 0 || status.CurrentFileBytes == 0 {
		t.Fatalf("recording status=%+v", status)
	}
	if _, err := os.Stat(dir + "/" + managedLogFileName + ".3"); err != nil {
		t.Fatalf("third backup missing: %v", err)
	}
	if _, err := os.Stat(dir + "/" + managedLogFileName + ".4"); !os.IsNotExist(err) {
		t.Fatalf("more than three backups retained")
	}
	recorder.Close()
	before := recorder.Status().TotalBytes
	time.Sleep(100 * time.Millisecond)
	after := recorder.Status()
	if after.Enabled || after.TotalBytes != before {
		t.Fatalf("disabled status changed: before=%d after=%+v", before, after)
	}
}

// Exercise a real HTTP connection: a recorder does not reveal buffered headers.
func TestManagerQuietLogStreamFlushesHeadersAndCancels(t *testing.T) {
	useTestConfigDir(t)
	disconnected := make(chan struct{})
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/logs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(disconnected)
	}))
	defer core.Close()
	cfg := *GlobalConfig()
	cfg.Mihomo.ExternalController = strings.TrimPrefix(core.URL, "http://")
	SetGlobalConfig(cfg)
	manager := httptest.NewServer((&Daemon{}).router())
	defer manager.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(manager.URL + "/v1/logs/stream?level=info")
	if err != nil {
		t.Fatalf("quiet stream did not send response headers: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("quiet stream status = %d", response.StatusCode)
	}
	response.Body.Close()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("closing the viewer did not cancel the quiet core subscription")
	}
}
