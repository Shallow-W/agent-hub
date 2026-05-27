package middleware

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidateUUIDParam 校验路径参数是否为合法 UUID，否则返回 400
func ValidateUUIDParam(param string) gin.HandlerFunc {
	return func(c *gin.Context) {
		val := c.Param(param)
		if val != "" && !uuidPattern.MatchString(val) {
			ErrorResponse(c, http.StatusBadRequest, 40040, "无效 ID 格式")
			c.Abort()
			return
		}
		c.Next()
	}
}

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// SuccessResponse 成功响应
func SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// CreatedResponse 资源创建成功响应
func CreatedResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// ErrorResponse 错误响应
func ErrorResponse(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}
