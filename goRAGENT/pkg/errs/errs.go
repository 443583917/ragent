// Package errs 提供统一错误类型，对齐 docs/development-standards.md 第三节。
//
// 错误分级（分级错误类型）：
//   - A 类：客户端错误（未登录/无权限）
//   - B 类：业务错误（参数错误/业务规则不满足）
//   - C 类：第三方远程服务错误
//   - S 类：服务端内部错误
//
// 使用方式：
//
//	return errs.NotFound("知识库不存在")                  // 业务层直接构造
//	return errs.WrapServer(err, "查询知识库列表失败")       // 包装底层 error
//	errs.CodeOf(err)                                     // handler 层取错误码渲染响应
package errs

import (
	"errors"
	"fmt"
)

// 标准错误码（和 pkg/response 错误码定义对应）
const (
	CodeNotLogin      = "A000001" // 未登录
	CodeForbidden     = "A000002" // 无权限
	CodeParamError    = "B000001" // 参数错误
	CodeBusinessError = "B000002" // 业务错误
	CodeNotFound      = "B000003" // 资源不存在
	CodeRemoteError   = "C000001" // 远程服务错误
	CodeServerError   = "S000001" // 服务器内部错误
)

// AppError 统一应用错误。
// Code 用于前端分支判断，Message 面向用户展示，Cause 保留原始错误链（不暴露给前端）。
type AppError struct {
	Code    string // 错误码，如 "B000002"
	Message string // 用户可读描述
	Cause   error  // 原始 error，可为 nil
}

// Error 实现 error 接口，携带完整错误链便于日志排查。
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is / errors.As 沿错误链匹配。
func (e *AppError) Unwrap() error { return e.Cause }

// New 构造指定错误码的 AppError。
func New(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Wrap 包装底层错误为指定错误码的 AppError；err 为 nil 时返回 nil。
func Wrap(err error, code, message string) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{Code: code, Message: message, Cause: err}
}

// ---- 常用构造（业务层直接使用，避免手写错误码） ----

// NotLogin 未登录错误。
func NotLogin(message string) *AppError { return New(CodeNotLogin, message) }

// Forbidden 无权限错误。
func Forbidden(message string) *AppError { return New(CodeForbidden, message) }

// Param 参数校验错误。
func Param(message string) *AppError { return New(CodeParamError, message) }

// Business 业务规则错误。
func Business(message string) *AppError { return New(CodeBusinessError, message) }

// NotFound 资源不存在错误。
func NotFound(message string) *AppError { return New(CodeNotFound, message) }

// WrapBusiness 包装底层错误为业务错误。
func WrapBusiness(err error, message string) *AppError {
	return Wrap(err, CodeBusinessError, message)
}

// WrapRemote 包装第三方服务调用错误。
func WrapRemote(err error, message string) *AppError {
	return Wrap(err, CodeRemoteError, message)
}

// WrapServer 包装服务端内部错误（DB/缓存等基础设施故障）。
func WrapServer(err error, message string) *AppError {
	return Wrap(err, CodeServerError, message)
}

// ---- handler 层辅助 ----

// CodeOf 提取错误链中的错误码；非 AppError 一律视为服务器内部错误。
func CodeOf(err error) string {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return CodeServerError
}

// MessageOf 提取错误链中面向用户的描述；非 AppError 返回兜底文案，避免内部细节泄露。
func MessageOf(err error) string {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Message
	}
	return "服务器内部错误"
}

// Is 判断错误链中是否存在指定错误码的 AppError。
func Is(err error, code string) bool {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Code == code
	}
	return false
}
