// Package response 提供统一 HTTP 响应体（统一响应体）。
package response

import (
	"encoding/json"
	"net/http"

	"goRAGENT/pkg/errs"
)

// Result 统一响应体（统一响应体）
type Result struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// 标准错误码（语义定义收敛在 pkg/errs）
const (
	CodeSuccess       = "0"
	CodeNotLogin      = errs.CodeNotLogin
	CodeForbidden     = errs.CodeForbidden
	CodeParamError    = errs.CodeParamError
	CodeBusinessError = errs.CodeBusinessError
	CodeNotFound      = errs.CodeNotFound
	CodeRemoteError   = errs.CodeRemoteError
	CodeServerError   = errs.CodeServerError
)

// Success 成功响应
func Success(data any) *Result {
	return &Result{Code: CodeSuccess, Data: data}
}

// SuccessOK 成功无数据响应
func SuccessOK() *Result {
	return &Result{Code: CodeSuccess}
}

// Failure 业务错误
func Failure(code, message string) *Result {
	return &Result{Code: code, Message: message}
}

// FromError 将任意 error 转换为统一响应体。
// AppError 携带的错误码/文案原样透出；未知 error 统一渲染为服务器内部错误，避免泄露内部细节。
func FromError(err error) *Result {
	return &Result{Code: errs.CodeOf(err), Message: errs.MessageOf(err)}
}

// WriteJSON 直接写入 HTTP JSON 响应（非 Gin 场景，如原生 http.Handler / SSE 前置错误）
func WriteJSON(w http.ResponseWriter, statusCode int, r *Result) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(r)
}
