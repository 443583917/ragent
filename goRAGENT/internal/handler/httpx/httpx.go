// Package httpx 提供 handler 层公共的参数解析与响应渲染函数。
package httpx

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"goRAGENT/internal/model"
	"goRAGENT/pkg/response"
)

// PageFromQuery 解析 page/pageSize 风格分页参数（非法值回退默认）。
func PageFromQuery(c *gin.Context) model.PageQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(model.DefaultPage)))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", strconv.Itoa(model.DefaultPageSize)))
	return model.PageQuery{Page: page, Size: size}.Normalize()
}

// PageFromCurrentSize 解析 current/size 风格分页参数（对齐前端 PageResult<T> 端点）。
func PageFromCurrentSize(c *gin.Context) model.PageQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("current", strconv.Itoa(model.DefaultPage)))
	size, _ := strconv.Atoi(c.DefaultQuery("size", strconv.Itoa(model.DefaultPageSize)))
	return model.PageQuery{Page: page, Size: size}.Normalize()
}

// OK 渲染成功响应。
func OK(c *gin.Context, data any) { c.JSON(http.StatusOK, response.Success(data)) }

// OKEmpty 渲染无数据成功响应。
func OKEmpty(c *gin.Context) { c.JSON(http.StatusOK, response.SuccessOK()) }

// Error 将 service 层错误渲染为统一响应（HTTP 200 + 业务错误码，保持现有前端契约）。
func Error(c *gin.Context, err error) { c.JSON(http.StatusOK, response.FromError(err)) }

// BadRequest 参数绑定失败的快捷渲染。
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusOK, response.Failure(response.CodeParamError, message))
}
