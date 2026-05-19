package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

type lokiHookConfig struct {
	URL           string
	Username      string
	Password      string
	Project       string
	Service       string
	Environment   string
	Cluster       string
	BatchSize     int
	QueueSize     int
	FlushInterval time.Duration
	Timeout       time.Duration
	ErrorOutput   io.Writer
}

type lokiHook struct {
	url           string
	username      string
	password      string
	project       string
	service       string
	environment   string
	cluster       string
	batchSize     int
	queue         chan lokiRecord
	flushInterval time.Duration
	client        *http.Client
	errorOutput   io.Writer
	done          chan struct{}
	closeOnce     sync.Once
	wg            sync.WaitGroup
	closed        atomic.Bool
	dropped       atomic.Uint64
}

type lokiRecord struct {
	labels map[string]string
	value  []json.RawMessage
}

type lokiPushRequest struct {
	Streams []lokiPushStream `json:"streams"`
}

type lokiPushStream struct {
	Stream map[string]string   `json:"stream"`
	Values [][]json.RawMessage `json:"values"`
}

var lokiStructuredMetadataFields = map[string]string{
	"request_id":  "request_id",
	"search_id":   "search_id",
	"job_id":      "job_id",
	"path":        "path",
	"full_path":   "full_path",
	"query":       "query",
	"method":      "method",
	"status":      "status",
	"duration_ms": "duration_ms",
	"size":        "size",
	"client_ip":   "client_ip",
	"source":      "source",
	"error":       "error_msg",
}

func newLokiHook(cfg lokiHookConfig) *lokiHook {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 5000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.Project == "" {
		cfg.Project = "emomo"
	}
	if cfg.ErrorOutput == nil {
		cfg.ErrorOutput = os.Stderr
	}

	h := &lokiHook{
		url:           cfg.URL,
		username:      cfg.Username,
		password:      cfg.Password,
		project:       cfg.Project,
		service:       cfg.Service,
		environment:   cfg.Environment,
		cluster:       cfg.Cluster,
		batchSize:     cfg.BatchSize,
		queue:         make(chan lokiRecord, cfg.QueueSize),
		flushInterval: cfg.FlushInterval,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		errorOutput: cfg.ErrorOutput,
		done:        make(chan struct{}),
	}

	h.wg.Add(1)
	go h.run()

	return h
}

func (h *lokiHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *lokiHook) Fire(entry *logrus.Entry) error {
	if h.closed.Load() {
		return nil
	}

	record, err := h.recordFromEntry(entry)
	if err != nil {
		return err
	}

	select {
	case h.queue <- record:
	default:
		h.dropped.Add(1)
	}

	return nil
}

func (h *lokiHook) Close() error {
	h.closeOnce.Do(func() {
		h.closed.Store(true)
		close(h.done)
	})
	h.wg.Wait()
	return nil
}

func (h *lokiHook) run() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	batch := make([]lokiRecord, 0, h.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := h.push(batch); err != nil && h.errorOutput != nil {
			fmt.Fprintf(h.errorOutput, "loki push failed: %v\n", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case record := <-h.queue:
			batch = append(batch, record)
			if len(batch) >= h.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-h.done:
			for {
				select {
				case record := <-h.queue:
					batch = append(batch, record)
					if len(batch) >= h.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (h *lokiHook) recordFromEntry(entry *logrus.Entry) (lokiRecord, error) {
	data := make(logrus.Fields, len(entry.Data))
	for key, value := range entry.Data {
		data[key] = value
	}

	lineFields := make(map[string]interface{}, len(data)+5)
	lineFields["timestamp"] = entry.Time.Format("2006-01-02T15:04:05.000Z07:00")
	lineFields["level"] = entry.Level.String()
	lineFields["message"] = entry.Message
	for key, value := range data {
		lineFields[key] = normalizeLogValue(value)
	}
	if entry.Caller != nil {
		lineFields["func"] = entry.Caller.Function
		lineFields["file"] = fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)
	}

	lineBytes, err := json.Marshal(lineFields)
	if err != nil {
		for key, value := range data {
			lineFields[key] = stringifyLogValue(value)
		}
		lineBytes, err = json.Marshal(lineFields)
		if err != nil {
			return lokiRecord{}, err
		}
	}

	metadata := make(map[string]string)
	for sourceKey, targetKey := range lokiStructuredMetadataFields {
		if value, ok := data[sourceKey]; ok {
			if stringValue := stringifyLogValue(value); stringValue != "" {
				metadata[targetKey] = stringValue
			}
		}
	}

	timestamp, err := json.Marshal(strconv.FormatInt(entry.Time.UnixNano(), 10))
	if err != nil {
		return lokiRecord{}, err
	}
	line, err := json.Marshal(string(lineBytes))
	if err != nil {
		return lokiRecord{}, err
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return lokiRecord{}, err
	}

	return lokiRecord{
		labels: h.labelsForEntry(entry, data),
		value:  []json.RawMessage{timestamp, line, metadataBytes},
	}, nil
}

func (h *lokiHook) labelsForEntry(entry *logrus.Entry, data logrus.Fields) map[string]string {
	labels := make(map[string]string, 6)
	addLabel(labels, "project", h.project)
	addLabel(labels, "service", firstNonEmptyString(stringifyLogValue(data["service"]), h.service))
	addLabel(labels, "environment", h.environment)
	addLabel(labels, "cluster", h.cluster)
	addLabel(labels, "level", entry.Level.String())
	addLabel(labels, "component", stringifyLogValue(data["component"]))
	return labels
}

func (h *lokiHook) push(batch []lokiRecord) error {
	payload := lokiPushRequest{
		Streams: make([]lokiPushStream, 0, len(batch)),
	}
	streamIndexes := make(map[string]int)

	for _, record := range batch {
		key := labelsKey(record.labels)
		index, ok := streamIndexes[key]
		if !ok {
			index = len(payload.Streams)
			streamIndexes[key] = index
			payload.Streams = append(payload.Streams, lokiPushStream{
				Stream: record.labels,
				Values: make([][]json.RawMessage, 0, 1),
			})
		}
		payload.Streams[index].Values = append(payload.Streams[index].Values, record.value)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.username != "" || h.password != "" {
		req.SetBasicAuth(h.username, h.password)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("loki returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	return nil
}

func addLabel(labels map[string]string, key string, value string) {
	if value != "" {
		labels[key] = value
	}
}

func labelsKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(labels[key])
		builder.WriteByte('\xff')
	}
	return builder.String()
}

func normalizeLogValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	if err, ok := value.(error); ok {
		return err.Error()
	}
	return value
}

func stringifyLogValue(value interface{}) string {
	switch v := normalizeLogValue(value).(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
