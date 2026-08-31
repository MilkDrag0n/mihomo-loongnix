package mihomotui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const managedLogFileName = "mihomo-runtime.log"

// LoggingStatus is authoritative manager state. CurrentFile refers to the
// active file while recording and the most recent file while disabled.
type LoggingStatus struct {
	Enabled          bool   `json:"enabled"`
	CurrentFile      string `json:"current_file,omitempty"`
	CurrentFileBytes int64  `json:"current_file_bytes"`
	TotalBytes       int64  `json:"total_bytes"`
	MaxFileBytes     int64  `json:"max_file_bytes"`
	MaxBackups       int    `json:"max_backups"`
	LastError        string `json:"last_error,omitempty"`
}

// ManagedLogRecorder owns the optional mihomo /logs subscription. It is the
// only component allowed to write managed runtime logs.
type ManagedLogRecorder struct {
	mu         sync.Mutex
	dir        string
	file       *os.File
	size       int64
	enabled    bool
	maxBytes   int64
	maxBackups int
	cancel     context.CancelFunc
	done       chan struct{}
	lastError  string
	api        func() (*MihomoAPI, error)
}

func NewManagedLogRecorder(dir string, api func() (*MihomoAPI, error)) *ManagedLogRecorder {
	return &ManagedLogRecorder{dir: dir, api: api}
}

func (r *ManagedLogRecorder) Apply(cfg ManagedLoggingConfig) error {
	if cfg.MaxFileBytes <= 0 {
		cfg.MaxFileBytes = 10 << 20
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 3
	}
	r.mu.Lock()
	r.maxBytes, r.maxBackups = cfg.MaxFileBytes, cfg.MaxBackups
	already := r.enabled == cfg.Enabled
	r.mu.Unlock()
	if already {
		return nil
	}
	if cfg.Enabled {
		return r.enable()
	}
	r.disable()
	return nil
}

func (r *ManagedLogRecorder) enable() error {
	r.mu.Lock()
	if r.enabled {
		r.mu.Unlock()
		return nil
	}
	if err := r.openLocked(); err != nil {
		r.lastError = err.Error()
		r.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	r.enabled = true
	r.lastError = ""
	done := r.done
	r.mu.Unlock()
	go r.recordLoop(ctx, done)
	return nil
}

func (r *ManagedLogRecorder) disable() {
	r.mu.Lock()
	if !r.enabled {
		r.mu.Unlock()
		return
	}
	cancel, done := r.cancel, r.done
	r.enabled = false
	r.cancel = nil
	r.done = nil
	if cancel != nil {
		cancel()
	}
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
	r.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (r *ManagedLogRecorder) Close() { r.disable() }

func (r *ManagedLogRecorder) recordLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		api, err := r.api()
		if err == nil {
			resp, streamErr := api.GetLogsStreamContext(ctx, "")
			if streamErr == nil {
				if resp.StatusCode >= http.StatusBadRequest {
					err = fmt.Errorf("Mihomo 日志接口返回 %s", resp.Status)
					_ = resp.Body.Close()
				} else {
					backoff = time.Second
					err = r.consume(ctx, resp.Body)
					_ = resp.Body.Close()
				}
			} else {
				err = streamErr
			}
		}
		if ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		r.lastError = RedactURLInText(fmt.Sprintf("日志流中断: %v", err))
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (r *ManagedLogRecorder) consume(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Mihomo's HTTP endpoint emits newline-delimited JSON while some
		// compatible builds/proxies frame it as SSE. Accept both, but persist
		// only the JSON payload so rotations have one stable format.
		if payload, ok := strings.CutPrefix(line, "data:"); ok {
			line = strings.TrimSpace(payload)
		}
		if line == "" {
			continue
		}
		if err := r.writeLine([]byte(line + "\n")); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return io.EOF
}

func (r *ManagedLogRecorder) writeLine(line []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled || r.file == nil {
		return context.Canceled
	}
	if r.size > 0 && r.size+int64(len(line)) > r.maxBytes {
		if err := r.rotateLocked(); err != nil {
			return err
		}
	}
	n, err := r.file.Write(line)
	r.size += int64(n)
	return err
}

func (r *ManagedLogRecorder) openLocked() error {
	if r.dir == "" {
		r.dir = filepath.Join(GetConfigDir(), "logs")
	}
	if err := os.MkdirAll(r.dir, 0750); err != nil {
		return fmt.Errorf("创建受管日志目录失败: %w", err)
	}
	if err := os.Chmod(r.dir, 0750); err != nil {
		return fmt.Errorf("设置受管日志目录权限失败: %w", err)
	}
	var serviceGID uint32
	if os.Geteuid() == 0 {
		gid, err := lookupIPCGroupGID(ipcAccessGroup)
		if err != nil {
			return err
		}
		serviceGID = gid
		if err := os.Chown(r.dir, 0, int(gid)); err != nil {
			return fmt.Errorf("设置受管日志目录属组失败: %w", err)
		}
	}
	path := filepath.Join(r.dir, managedLogFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("打开受管日志失败: %w", err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		_ = f.Close()
		return fmt.Errorf("设置受管日志文件权限失败: %w", err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, 0, int(serviceGID)); err != nil {
			_ = f.Close()
			return fmt.Errorf("设置受管日志文件属组失败: %w", err)
		}
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.file, r.size = f, info.Size()
	return nil
}

func (r *ManagedLogRecorder) rotateLocked() error {
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
	base := filepath.Join(r.dir, managedLogFileName)
	_ = os.Remove(fmt.Sprintf("%s.%d", base, r.maxBackups))
	for i := r.maxBackups - 1; i >= 1; i-- {
		oldName := fmt.Sprintf("%s.%d", base, i)
		newName := fmt.Sprintf("%s.%d", base, i+1)
		if err := os.Rename(oldName, newName); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(base, base+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	r.size = 0
	return r.openLocked()
}

func (r *ManagedLogRecorder) Status() LoggingStatus {
	r.mu.Lock()
	status := LoggingStatus{Enabled: r.enabled, MaxFileBytes: r.maxBytes, MaxBackups: r.maxBackups, LastError: r.lastError}
	dir := r.dir
	r.mu.Unlock()
	if status.MaxFileBytes <= 0 {
		status.MaxFileBytes = 10 << 20
	}
	if status.MaxBackups <= 0 {
		status.MaxBackups = 3
	}
	entries, _ := os.ReadDir(dir)
	type item struct {
		name string
		size int64
	}
	var files []item
	for _, entry := range entries {
		if entry.IsDir() || (entry.Name() != managedLogFileName && !strings.HasPrefix(entry.Name(), managedLogFileName+".")) {
			continue
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, item{entry.Name(), info.Size()})
			status.TotalBytes += info.Size()
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].name == managedLogFileName {
			return true
		}
		if files[j].name == managedLogFileName {
			return false
		}
		return files[i].name < files[j].name
	})
	if len(files) > 0 {
		status.CurrentFile, status.CurrentFileBytes = files[0].name, files[0].size
	}
	return status
}
