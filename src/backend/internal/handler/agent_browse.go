package handler

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// browseFileEntry 对应 daemon browseFiles zip action 返回的单个文件项。
// daemon 用 base64 编码文件内容以避免 JSON 内嵌二进制，这里解码后写进 zip。
type browseFileEntry struct {
	Path       string `json:"path"`
	ContentB64 string `json:"content_b64"`
	Size       int64  `json:"size"`
}

// browseZipPayload daemon zip action 的返回结构。
type browseZipPayload struct {
	BaseDir   string            `json:"baseDir"`
	Files     []browseFileEntry `json:"files"`
	Truncated bool              `json:"truncated"`
}

// BrowseFiles 浏览 agent 所在 daemon 机器上的文件。
// GET /api/agents/:id/files/browse?action=tree|list|read|zip&path=...&rev=...
//
// action=tree/list/read 时透传 daemon 返回的 JSON；
// action=zip 时 daemon 返回文件数组，这里用 archive/zip 打包后流式写回（零外部依赖）。
func (h *AgentHandler) BrowseFiles(c *gin.Context) {
	// files 参数：前端传逗号分隔字符串（status action 用），split 成数组
	var files []string
	if raw := c.Query("files"); raw != "" {
		for _, f := range strings.Split(raw, ",") {
			if f = strings.TrimSpace(f); f != "" {
				files = append(files, f)
			}
		}
	}
	req := service.BrowseRequest{
		Action:  c.Query("action"),
		Path:    c.Query("path"),
		Rev:     c.Query("rev"),
		WorkDir: c.Query("work_dir"),
		Files:   files,
	}
	userID := middleware.GetUserID(c)

	result, err := h.svc.BrowseAgentFiles(c.Request.Context(), userID, c.Param("id"), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAgentInvalidInput):
			middleware.ErrorResponse(c, http.StatusBadRequest, 40041, err.Error())
		case errors.Is(err, service.ErrAgentNotFound):
			middleware.ErrorResponse(c, http.StatusNotFound, 40434, err.Error())
		case errors.Is(err, service.ErrAgentOffline):
			middleware.ErrorResponse(c, http.StatusServiceUnavailable, 50301, "Agent 所在电脑未连接")
		case errors.Is(err, service.ErrMsgAgentTimeout):
			middleware.ErrorResponse(c, http.StatusGatewayTimeout, 50401, "浏览文件超时")
		default:
			middleware.ErrorResponse(c, http.StatusInternalServerError, 50040, "浏览文件失败")
		}
		return
	}

	// zip action 需要把 daemon 的文件数组打包成 zip 流；其余 action 直接透传 JSON。
	if req.Action == "zip" {
		serveBrowseZip(c, result)
		return
	}

	// result 是 json.RawMessage，作为 data 字段值会被 gin 正确内联为 JSON 对象/数组，
	// 而不是被二次字符串化。这样前端拿到的 data 直接是 daemon 的原始结构。
	middleware.SuccessResponse(c, *result)
}

// serveBrowseZip 解析 daemon 返回的 zip payload，流式写入 zip 响应。
// 用 archive/zip 标准库，避免引入外部打包依赖；文件内容 base64 解码后写入 zip entry。
func serveBrowseZip(c *gin.Context, raw *service.BrowseResult) {
	var payload browseZipPayload
	if err := json.Unmarshal(*raw, &payload); err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50041, "解析打包数据失败")
		return
	}

	baseName := sanitizeZipName(payload.BaseDir)
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="`+baseName+`.zip"`)
	c.Status(http.StatusOK)

	// zip.Writer 直接写 HTTP response body，不全 load 进内存。
	writer := zip.NewWriter(c.Writer)
	defer writer.Close()

	for _, f := range payload.Files {
		// 截断/不可读文件已在 daemon 端过滤；这里解码失败仅跳过该文件，保证其余能下载
		content, err := base64.StdEncoding.DecodeString(f.ContentB64)
		if err != nil {
			continue
		}
		w, err := writer.Create(f.Path)
		if err != nil {
			continue
		}
		_, _ = w.Write(content)
	}
}

// sanitizeZipName 从 baseDir 绝对路径提取一个安全的 zip 文件名。
// 取最后一段目录名，剥离非法字符；兜底 "agenthub-files"。
func sanitizeZipName(baseDir string) string {
	name := baseDir
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' || name[i] == '\\' {
			name = name[i+1:]
			break
		}
	}
	if name == "" {
		return "agenthub-files"
	}
	// 替换文件名非法字符
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '/' || ch == '\\' || ch == ':' || ch == '*' || ch == '?' || ch == '"' || ch == '<' || ch == '>' || ch == '|' {
			out = append(out, '_')
		} else {
			out = append(out, ch)
		}
	}
	result := string(out)
	if result == "." || result == ".." {
		return "agenthub-files"
	}
	return result
}
