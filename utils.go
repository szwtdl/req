package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
)

func ToString(value interface{}) string {
	return fmt.Sprintf("%v", value)
}

func IsTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func IsDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

func IsConnectionRefused(err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}
	var sysErr *os.SyscallError
	if errors.As(opErr.Err, &sysErr) {
		return errors.Is(sysErr.Err, syscall.ECONNREFUSED)
	}
	return false
}

func IsNetworkUnreachable(err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}
	var sysErr *os.SyscallError
	if errors.As(opErr.Err, &sysErr) {
		return errors.Is(sysErr.Err, syscall.ENETUNREACH)
	}
	return false
}

func IsInvalidAddressError(err error) bool {
	// 注意：http.Client.Do 会把所有错误包装成 *url.Error，
	// 所以不能直接用 url.Error 判断，否则会误判所有传输层错误。
	// 这里只匹配真正的地址解析错误。
	var addrErr *net.AddrError
	return errors.As(err, &addrErr)
}

// IsRetryableError 判断是否为可重试的瞬态连接错误。
// 适用场景：高并发下代理/服务端提前关闭连接、TLS 握手 EOF 等。
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// EOF：连接被对端提前关闭（代理在高并发 TLS 握手时断开）
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// connection reset by peer
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			if errors.Is(sysErr.Err, syscall.ECONNRESET) {
				return true
			}
		}
	}
	// 兜底：通过错误消息匹配常见瞬态错误
	msg := err.Error()
	for _, keyword := range []string{
		"EOF",
		"connection reset by peer",
		"use of closed network connection",
		"broken pipe",
	} {
		if strings.Contains(msg, keyword) {
			return true
		}
	}
	return false
}
