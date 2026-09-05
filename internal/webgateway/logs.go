package webgateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type logEvent struct {
	Level      string `json:"level"`
	Message    string `json:"message"`
	ReceivedAt string `json:"received_at"`
}
type streamItem struct {
	event string
	value any
}

func parseLog(line string) (logEvent, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	var raw struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(line), &raw) != nil {
		return logEvent{}, false
	}
	if raw.Level == "" {
		raw.Level = raw.Type
	}
	if raw.Message == "" {
		raw.Message = raw.Payload
	}
	if raw.Level == "" {
		return logEvent{}, false
	}
	return logEvent{raw.Level, raw.Message, time.Now().UTC().Format(time.RFC3339)}, true
}
func (s *Server) logs(w http.ResponseWriter, r *http.Request, id string, v *session) {
	level := r.URL.Query().Get("level")
	if level == "" {
		level = "info"
	}
	if !map[string]bool{"debug": true, "info": true, "warning": true, "error": true, "silent": true}[level] {
		failure(w, 400, "INVALID_INPUT", "日志级别无效")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://manager/v1/logs/stream?level="+url.QueryEscape(level), nil)
	res, e := s.stream.Do(req)
	if e != nil {
		failure(w, 502, "UPSTREAM_UNAVAILABLE", "日志暂时不可用")
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		failure(w, 502, "UPSTREAM_UNAVAILABLE", "日志暂时不可用")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	rc := http.NewResponseController(w)
	if rc.Flush() != nil {
		return
	}
	items := make(chan streamItem, 8)
	go func() {
		defer close(items)
		reader := bufio.NewReaderSize(res.Body, 64<<10)
		discard := false
		send := func(x streamItem) bool {
			select {
			case items <- x:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for {
			line, err := reader.ReadSlice('\n')
			if errors.Is(err, bufio.ErrBufferFull) {
				discard = true
				continue
			}
			if discard {
				discard = false
				if !send(streamItem{"gap", map[string]string{"reason": "event_too_large"}}) {
					return
				}
			} else if log, ok := parseLog(string(line)); ok {
				if !send(streamItem{"log", log}) {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					send(streamItem{"gap", map[string]string{"reason": "upstream_disconnected"}})
				}
				return
			}
		}
	}()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	expiry := time.NewTicker(time.Second)
	defer expiry.Stop()
	emit := func(event string, value any) error {
		data, _ := json.Marshal(value)
		_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, e := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		if e == nil {
			e = rc.Flush()
		}
		return e
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closed:
			return
		case <-v.Done:
			return
		case <-expiry.C:
			_, current := s.getSession(r)
			if current == nil {
				return
			}
		case <-heartbeat.C:
			_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, e := fmt.Fprint(w, ": heartbeat\n\n"); e != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		case item, ok := <-items:
			if !ok {
				_ = emit("gap", map[string]string{"reason": "upstream_reconnect_required"})
				return
			}
			if emit(item.event, item.value) != nil {
				return
			}
		}
	}
}
