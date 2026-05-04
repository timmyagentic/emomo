package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/timmy/emomo/internal/logger"
)

// AdminHandler handles admin operations.
type AdminHandler struct {
	logger *logger.Logger
}

// NewAdminHandler creates a new admin handler.
// Parameters:
//   - log: logger instance.
//
// Returns:
//   - *AdminHandler: initialized handler.
func NewAdminHandler(log *logger.Logger) *AdminHandler {
	return &AdminHandler{
		logger: log,
	}
}

// log returns a logger from Gin context if available, otherwise returns the default logger
func (h *AdminHandler) log(c *gin.Context) *logger.Logger {
	if l := logger.FromContext(c.Request.Context()); l != nil {
		return l
	}
	return h.logger
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

            <div class="stats">
                <div class="stats-row"><span>唯一导入入口</span><span>backend/scripts/import-data.sh</span></div>
                <div class="stats-row"><span>本地目录参数</span><span>-p / --path</span></div>
            </div>
            <pre style="margin-top: 1rem; padding: 1rem; background: #1f2937; color: #f9fafb; border-radius: 8px; overflow-x: auto;">cd backend
./scripts/import-data.sh -p ./data/memes --profile qwen3vl</pre>
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
</body>
</html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}
