package model

import "strings"

// IsCodexInterruptionText reports whether text looks like a Codex service or
// transport interruption rather than an ordinary tool failure.
func IsCodexInterruptionText(values ...string) bool {
	for _, value := range values {
		if isCodexInterruptionText(value) {
			return true
		}
	}
	return false
}

func isCodexInterruptionText(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return false
	}

	for _, code := range []string{"401", "402", "403", "429", "500", "502", "503", "504"} {
		if containsStatusCode(text, code) {
			return true
		}
	}

	markers := []string{
		"unauthorized",
		"forbidden",
		"authentication failed",
		"authentication error",
		"invalid api key",
		"invalid token",
		"payment required",
		"billing error",
		"billing required",
		"quota exhausted",
		"quota exceeded",
		"quota limit",
		"rate limit",
		"too many requests",
		"rate_limit",
		"ratelimit",
		"usage limit",
		"usage_limit",
		"credits exhausted",
		"credit exhausted",
		"insufficient credits",
		"out of credits",
		"limit reached",
		"token budget exhausted",
		"usage exhausted",
		"network error",
		"network failure",
		"network unavailable",
		"network connection",
		"connection refused",
		"connection reset",
		"connection closed",
		"connection lost",
		"connection timeout",
		"connect timeout",
		"request timeout",
		"timed out",
		"timeout",
		"dns",
		"socket",
		"tls handshake",
		"transport error",
		"unexpected eof",
		"service unavailable",
		"internal server error",
		"server error",
		"service error",
		"gateway timeout",
		"bad gateway",
		"server disconnected",
		"disconnected",
		"codex error",
		"codex interrupted",
		"turn failed",
		"turn aborted",
		"task failed",
		"turn interrupted",
		"认证失败",
		"未授权",
		"额度",
		"配额",
		"用光",
		"用完",
		"超额",
		"限额",
		"余额不足",
		"网络错误",
		"网络中断",
		"网络连接",
		"网络不可用",
		"连接失败",
		"连接中断",
		"连接断开",
		"超时",
		"服务不可用",
		"请求失败",
		"异常中断",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func containsStatusCode(text, code string) bool {
	start := 0
	for {
		index := strings.Index(text[start:], code)
		if index < 0 {
			return false
		}
		index += start
		beforeDigit := index == 0 || !isASCIIDigit(text[index-1])
		after := index + len(code)
		afterDigit := after == len(text) || !isASCIIDigit(text[after])
		if beforeDigit && afterDigit {
			return true
		}
		start = after
		if start >= len(text) {
			return false
		}
	}
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
