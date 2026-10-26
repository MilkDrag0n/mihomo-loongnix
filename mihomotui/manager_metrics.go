package mihomotui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// RuntimeMetrics contains aggregate counters only, never connection destinations,
// process names, controller credentials or per-connection metadata.
type RuntimeMetrics struct {
	ObservedAt    string `json:"observed_at"`
	DownloadTotal *int64 `json:"download_total"`
	UploadTotal   *int64 `json:"upload_total"`
	MemoryBytes   *int64 `json:"memory_bytes"`
	Connections   *int   `json:"connections"`
}

func runtimeMetrics(api *MihomoAPI) *RuntimeMetrics {
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	req, err := api.buildRequest(http.MethodGet, "/connections", nil, nil)
	if err != nil {
		return nil
	}
	res, err := api.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	var source struct {
		DownloadTotal *int64          `json:"downloadTotal"`
		UploadTotal   *int64          `json:"uploadTotal"`
		MemoryBytes   *int64          `json:"memory"`
		Connections   json.RawMessage `json:"connections"`
	}
	// Bound controller responses; decoding cannot leak raw data to callers.
	if json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&source) != nil {
		return nil
	}
	metric := &RuntimeMetrics{ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), DownloadTotal: source.DownloadTotal, UploadTotal: source.UploadTotal, MemoryBytes: nil}
	for _, n := range []*int64{metric.DownloadTotal, metric.UploadTotal, metric.MemoryBytes} {
		if n != nil && *n < 0 {
			return nil
		}
	}
	if len(source.Connections) > 0 {
		var connections []json.RawMessage
		if json.Unmarshal(source.Connections, &connections) != nil {
			return nil
		}
		count := len(connections)
		metric.Connections = &count
	}
	if metric.DownloadTotal == nil && metric.UploadTotal == nil && metric.MemoryBytes == nil && metric.Connections == nil {
		return nil
	}
	if req, e := api.buildRequest(http.MethodGet, "/memory", nil, nil); e == nil {
		if res, e := api.client.Do(req.WithContext(ctx)); e == nil {
			defer res.Body.Close()
			var memory struct {
				Inuse *int64 `json:"inuse"`
			}
			// Mihomo deliberately emits a zero placeholder in the first /memory frame.
			// Read the second frame within the bounded request, never report the placeholder.
			decoder := json.NewDecoder(io.LimitReader(res.Body, 4096))
			if res.StatusCode == http.StatusOK && decoder.Decode(&memory) == nil && decoder.Decode(&memory) == nil && memory.Inuse != nil && *memory.Inuse >= 0 {
				metric.MemoryBytes = memory.Inuse
			}
		}
	}
	return metric
}
