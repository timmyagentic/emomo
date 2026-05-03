package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	pb "github.com/timmy/emomo/gen/emomo/v1"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/source"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AdminHandler handles admin operations.
type AdminHandler struct {
	ingestService *service.IngestService
	sources       map[string]source.Source
	logger        *logger.Logger

	// Ingest job state
	mu            sync.RWMutex
	isRunning     bool
	currentStats  *service.IngestStats
	lastRunTime   time.Time
	lastRunStatus string
}

// NewAdminHandler creates a new admin handler.
// Parameters:
//   - ingestService: ingest service instance.
//   - sources: map of source adapters keyed by name.
//   - log: logger instance.
//
// Returns:
//   - *AdminHandler: initialized handler.
func NewAdminHandler(ingestService *service.IngestService, sources map[string]source.Source, log *logger.Logger) *AdminHandler {
	return &AdminHandler{
		ingestService: ingestService,
		sources:       sources,
		logger:        log,
	}
}

// log returns a logger from Gin context if available, otherwise returns the default logger
func (h *AdminHandler) log(c *gin.Context) *logger.Logger {
	if l := logger.FromContext(c.Request.Context()); l != nil {
		return l
	}
	return h.logger
}

// ingestStatsToPb projects the service-internal IngestStats into the
// wire-shape protobuf message embedded in trigger / status responses.
func ingestStatsToPb(s *service.IngestStats) *pb.IngestStats {
	if s == nil {
		return nil
	}
	out := &pb.IngestStats{
		TotalItems:     s.TotalItems,
		ProcessedItems: s.ProcessedItems,
		SkippedItems:   s.SkippedItems,
		FailedItems:    s.FailedItems,
	}
	if !s.StartTime.IsZero() {
		out.StartTime = timestamppb.New(s.StartTime)
	}
	if !s.EndTime.IsZero() {
		out.EndTime = timestamppb.New(s.EndTime)
	}
	return out
}

// AdminPage serves the admin dashboard HTML page.
// Parameters:
//   - c: Gin request context.
//
// Returns: none (writes HTML response).
func (h *AdminHandler) AdminPage(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Emomo Admin</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 2rem;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
        }
        .card {
            background: white;
            border-radius: 16px;
            padding: 2rem;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            margin-bottom: 1.5rem;
        }
        h1 {
            color: #333;
            margin-bottom: 0.5rem;
            font-size: 1.8rem;
        }
        .subtitle {
            color: #666;
            margin-bottom: 1.5rem;
        }
        .form-group {
            margin-bottom: 1rem;
        }
        label {
            display: block;
            margin-bottom: 0.5rem;
            color: #444;
            font-weight: 500;
        }
        select, input[type="number"] {
            width: 100%;
            padding: 0.75rem;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            font-size: 1rem;
            transition: border-color 0.2s;
        }
        select:focus, input:focus {
            outline: none;
            border-color: #667eea;
        }
        .checkbox-group {
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        .checkbox-group input {
            width: 18px;
            height: 18px;
        }
        button {
            width: 100%;
            padding: 1rem;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            border-radius: 8px;
            font-size: 1.1rem;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        button:hover:not(:disabled) {
            transform: translateY(-2px);
            box-shadow: 0 5px 20px rgba(102, 126, 234, 0.4);
        }
        button:disabled {
            opacity: 0.6;
            cursor: not-allowed;
        }
        .status {
            padding: 1rem;
            border-radius: 8px;
            margin-top: 1rem;
            display: none;
        }
        .status.success {
            background: #d4edda;
            color: #155724;
            display: block;
        }
        .status.error {
            background: #f8d7da;
            color: #721c24;
            display: block;
        }
        .status.running {
            background: #fff3cd;
            color: #856404;
            display: block;
        }
        .stats {
            margin-top: 1rem;
            padding: 1rem;
            background: #f8f9fa;
            border-radius: 8px;
        }
        .stats-row {
            display: flex;
            justify-content: space-between;
            padding: 0.5rem 0;
            border-bottom: 1px solid #e0e0e0;
        }
        .stats-row:last-child {
            border-bottom: none;
        }
        .quick-links {
            display: flex;
            gap: 1rem;
            flex-wrap: wrap;
        }
        .quick-links a {
            flex: 1;
            min-width: 120px;
            padding: 0.75rem;
            background: #f8f9fa;
            color: #333;
            text-decoration: none;
            border-radius: 8px;
            text-align: center;
            transition: background 0.2s;
        }
        .quick-links a:hover {
            background: #e9ecef;
        }
        .spinner {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid #ffffff;
            border-radius: 50%;
            border-top-color: transparent;
            animation: spin 1s linear infinite;
            margin-right: 8px;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="card">
            <h1>🎭 Emomo Admin</h1>
            <p class="subtitle">表情包语义搜索系统管理面板</p>

            <form id="ingestForm">
	                <div class="form-group">
	                    <label for="source">数据源</label>
	                    <select id="source" name="source">
	                        <option value="localdir">本地静态图片目录</option>
	                    </select>
	                </div>

                <div class="form-group">
                    <label for="limit">导入数量</label>
                    <input type="number" id="limit" name="limit" value="100" min="1" max="10000">
                </div>

                <div class="form-group">
                    <div class="checkbox-group">
                        <input type="checkbox" id="force" name="force">
                        <label for="force" style="margin: 0;">强制重新处理（跳过重复检查）</label>
                    </div>
                </div>

                <button type="submit" id="submitBtn">
                    开始导入
                </button>
            </form>

            <div id="status" class="status"></div>
            <div id="stats" class="stats" style="display: none;"></div>
        </div>

        <div class="card">
            <h2 style="margin-bottom: 1rem;">快速链接</h2>
            <div class="quick-links">
                <a href="/api/v1/stats">📊 系统统计</a>
                <a href="/api/v1/categories">📁 分类列表</a>
                <a href="/api/v1/memes?limit=10">🖼️ 表情包</a>
                <a href="/health">💚 健康检查</a>
            </div>
        </div>
    </div>

    <script>
        const form = document.getElementById('ingestForm');
        const submitBtn = document.getElementById('submitBtn');
        const statusDiv = document.getElementById('status');
        const statsDiv = document.getElementById('stats');

        form.addEventListener('submit', async (e) => {
            e.preventDefault();

            const source = document.getElementById('source').value;
            const limit = parseInt(document.getElementById('limit').value);
            const force = document.getElementById('force').checked;

            submitBtn.disabled = true;
            submitBtn.innerHTML = '<span class="spinner"></span>导入中...';
            statusDiv.className = 'status running';
            statusDiv.textContent = '正在导入数据，请稍候...';
            statsDiv.style.display = 'none';

            try {
                const response = await fetch('/api/v1/ingest', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ source, limit, force })
                });

                const data = await response.json();

                if (response.ok) {
                    statusDiv.className = 'status success';
                    statusDiv.textContent = '✓ ' + data.message;

                    if (data.stats) {
                        statsDiv.style.display = 'block';
                        statsDiv.innerHTML = ` + "`" + `
                            <div class="stats-row"><span>总计</span><span>${data.stats.total_items ?? 0}</span></div>
                            <div class="stats-row"><span>已处理</span><span>${data.stats.processed_items ?? 0}</span></div>
                            <div class="stats-row"><span>跳过</span><span>${data.stats.skipped_items ?? 0}</span></div>
                            <div class="stats-row"><span>失败</span><span>${data.stats.failed_items ?? 0}</span></div>
                        ` + "`" + `;
                    }
                } else {
                    statusDiv.className = 'status error';
                    statusDiv.textContent = '✗ ' + (data.error || '导入失败');
                }
            } catch (err) {
                statusDiv.className = 'status error';
                statusDiv.textContent = '✗ 网络错误: ' + err.message;
            } finally {
                submitBtn.disabled = false;
                submitBtn.textContent = '开始导入';
            }
        });
    </script>
</body>
</html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// TriggerIngest handles POST /api/v1/ingest.
func (h *AdminHandler) TriggerIngest(c *gin.Context) {
	ctx := c.Request.Context()

	req := &pb.TriggerIngestRequest{}
	if err := readProtoJSON(c, req); err != nil {
		logger.CtxWarn(ctx, "Invalid ingest request: client_ip=%s, error=%v", c.ClientIP(), err)
		writeError(c, http.StatusBadRequest, err)
		return
	}
	if req.GetSource() == "" {
		writeError(c, http.StatusBadRequest, fmt.Errorf("source is required"))
		return
	}
	if req.GetLimit() < 1 || req.GetLimit() > 10000 {
		writeError(c, http.StatusBadRequest, fmt.Errorf("limit must be between 1 and 10000"))
		return
	}

	logger.CtxInfo(ctx, "Received ingest request: source=%s, limit=%d, force=%v, client_ip=%s",
		req.GetSource(), req.GetLimit(), req.GetForce(), c.ClientIP())

	h.mu.RLock()
	if h.isRunning {
		h.mu.RUnlock()
		logger.CtxWarn(ctx, "Ingest request rejected: already running, source=%s, client_ip=%s",
			req.GetSource(), c.ClientIP())
		writeError(c, http.StatusConflict, fmt.Errorf("ingest is already running"))
		return
	}
	h.mu.RUnlock()

	src, ok := h.sources[req.GetSource()]
	if !ok {
		logger.CtxWarn(ctx, "Unknown source requested: source=%s, client_ip=%s", req.GetSource(), c.ClientIP())
		writeError(c, http.StatusBadRequest, fmt.Errorf("unknown source: %s", req.GetSource()))
		return
	}

	h.mu.Lock()
	h.isRunning = true
	h.currentStats = nil
	h.mu.Unlock()

	logger.CtxInfo(ctx, "Starting ingest process: source=%s, limit=%d, force=%v",
		req.GetSource(), req.GetLimit(), req.GetForce())

	// Run ingest with a background context so an HTTP timeout doesn't cancel
	// the long-running pipeline.
	ingestCtx := context.Background()
	startTime := time.Now()
	stats, err := h.ingestService.IngestFromSource(ingestCtx, src, int(req.GetLimit()), &service.IngestOptions{
		Force: req.GetForce(),
	})
	duration := time.Since(startTime)

	h.mu.Lock()
	h.isRunning = false
	h.currentStats = stats
	h.lastRunTime = time.Now()
	if err != nil {
		h.lastRunStatus = "failed: " + err.Error()
	} else {
		h.lastRunStatus = "success"
	}
	h.mu.Unlock()

	if err != nil {
		logger.With(logger.Fields{
			logger.FieldDurationMs: duration.Milliseconds(),
		}).Error(ctx, "Ingest process failed: source=%s, limit=%d, force=%v, error=%v",
			req.GetSource(), req.GetLimit(), req.GetForce(), err)
		writeError(c, http.StatusInternalServerError, err)
		return
	}

	logger.With(logger.Fields{
		logger.FieldDurationMs: duration.Milliseconds(),
		logger.FieldCount:      stats.ProcessedItems,
	}).Info(ctx, "Ingest process completed: source=%s, total=%d, processed=%d, skipped=%d, failed=%d",
		req.GetSource(), stats.TotalItems, stats.ProcessedItems, stats.SkippedItems, stats.FailedItems)

	writeProtoJSON(c, http.StatusOK, &pb.TriggerIngestResponse{
		Message: "Ingest completed successfully",
		Stats:   ingestStatsToPb(stats),
	})
}

// GetIngestStatus handles GET /api/v1/ingest/status.
func (h *AdminHandler) GetIngestStatus(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ctx := c.Request.Context()
	logger.CtxDebug(ctx, "Ingest status requested: client_ip=%s, is_running=%v", c.ClientIP(), h.isRunning)

	resp := &pb.GetIngestStatusResponse{
		IsRunning:     h.isRunning,
		LastRunStatus: h.lastRunStatus,
		CurrentStats:  ingestStatsToPb(h.currentStats),
	}
	if !h.lastRunTime.IsZero() {
		resp.LastRunTime = timestamppb.New(h.lastRunTime)
	}
	writeProtoJSON(c, http.StatusOK, resp)
}
