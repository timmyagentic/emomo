package handler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/timmy/emomo/internal/logger"
	"github.com/timmy/emomo/internal/service"
	"github.com/timmy/emomo/internal/source"
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

// IngestRequest represents the ingest API request.
type IngestRequest struct {
	Source string `json:"source" binding:"required"`
	Limit  int    `json:"limit" binding:"required,min=1,max=10000"`
	Force  bool   `json:"force"`
}

// IngestResponse represents the ingest API response.
type IngestResponse struct {
	Message string               `json:"message"`
	Stats   *service.IngestStats `json:"stats,omitempty"`
}

// IngestStatusResponse represents the ingest status.
type IngestStatusResponse struct {
	IsRunning     bool                 `json:"is_running"`
	LastRunTime   string               `json:"last_run_time,omitempty"`
	LastRunStatus string               `json:"last_run_status,omitempty"`
	CurrentStats  *service.IngestStats `json:"current_stats,omitempty"`
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
                            <div class="stats-row"><span>总计</span><span>${data.stats.TotalItems}</span></div>
                            <div class="stats-row"><span>已处理</span><span>${data.stats.ProcessedItems}</span></div>
                            <div class="stats-row"><span>跳过</span><span>${data.stats.SkippedItems}</span></div>
                            <div class="stats-row"><span>失败</span><span>${data.stats.FailedItems}</span></div>
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

// TriggerIngest handles the ingest API endpoint.
// Parameters:
//   - c: Gin request context.
//
// Returns: none (writes JSON response).
func (h *AdminHandler) TriggerIngest(c *gin.Context) {
	ctx := c.Request.Context()

	var req IngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.CtxWarn(ctx, "Invalid ingest request: client_ip=%s, error=%v", c.ClientIP(), err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logger.CtxInfo(ctx, "Received ingest request: source=%s, limit=%d, force=%v, client_ip=%s",
		req.Source, req.Limit, req.Force, c.ClientIP())

	// Check if ingest is already running
	h.mu.RLock()
	if h.isRunning {
		h.mu.RUnlock()
		logger.CtxWarn(ctx, "Ingest request rejected: already running, source=%s, client_ip=%s",
			req.Source, c.ClientIP())
		c.JSON(http.StatusConflict, gin.H{"error": "Ingest is already running"})
		return
	}
	h.mu.RUnlock()

	// Get source
	src, ok := h.sources[req.Source]
	if !ok {
		logger.CtxWarn(ctx, "Unknown source requested: source=%s, client_ip=%s", req.Source, c.ClientIP())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown source: " + req.Source})
		return
	}

	// Set running state
	h.mu.Lock()
	h.isRunning = true
	h.currentStats = nil
	h.mu.Unlock()

	logger.CtxInfo(ctx, "Starting ingest process: source=%s, limit=%d, force=%v",
		req.Source, req.Limit, req.Force)

	// Run ingest (use background context to avoid cancellation on HTTP timeout)
	ingestCtx := context.Background()
	startTime := time.Now()
	stats, err := h.ingestService.IngestFromSource(ingestCtx, src, req.Limit, &service.IngestOptions{
		Force: req.Force,
	})
	duration := time.Since(startTime)

	// Update state
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
			req.Source, req.Limit, req.Force, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.With(logger.Fields{
		logger.FieldDurationMs: duration.Milliseconds(),
		logger.FieldCount:      stats.ProcessedItems,
	}).Info(ctx, "Ingest process completed: source=%s, total=%d, processed=%d, skipped=%d, failed=%d",
		req.Source, stats.TotalItems, stats.ProcessedItems, stats.SkippedItems, stats.FailedItems)

	c.JSON(http.StatusOK, IngestResponse{
		Message: "Ingest completed successfully",
		Stats:   stats,
	})
}

// GetIngestStatus returns the current ingest status.
// Parameters:
//   - c: Gin request context.
//
// Returns: none (writes JSON response).
func (h *AdminHandler) GetIngestStatus(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ctx := c.Request.Context()
	logger.CtxDebug(ctx, "Ingest status requested: client_ip=%s, is_running=%v", c.ClientIP(), h.isRunning)

	resp := IngestStatusResponse{
		IsRunning:     h.isRunning,
		LastRunStatus: h.lastRunStatus,
		CurrentStats:  h.currentStats,
	}

	if !h.lastRunTime.IsZero() {
		resp.LastRunTime = h.lastRunTime.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, resp)
}
