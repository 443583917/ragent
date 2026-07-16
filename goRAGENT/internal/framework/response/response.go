package response

import "net/http"

// Result 统一响应体（和 Java Result 对应）
type Result struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// 标准错误码（和 Java BaseErrorCode 对应）
const (
	CodeSuccess       = "0"
	CodeNotLogin      = "A000001"
	CodeForbidden     = "A000002"
	CodeParamError    = "B000001"
	CodeBusinessError = "B000002"
	CodeRemoteError   = "C000001"
	CodeServerError   = "S000001"
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

// WriteJSON 直接写入 HTTP JSON 响应（非 SSE 场景）
func WriteJSON(w http.ResponseWriter, statusCode int, r *Result) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.WriteHeader(statusCode)
	// 由 Gin 处理序列化，此处仅提供结构体
}
