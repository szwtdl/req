package client

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
)

// SetProxy 配置代理（nil 表示清除代理）。支持 "http" 和 "socks5" 两种类型。
func (h *HttpClient) SetProxy(cfg *ProxyConfig) error {
	if cfg == nil {
		h.transport.Proxy = nil
		h.transport.DialContext = nil
		return nil
	}

	switch cfg.Type {
	case "http":
		proxyURL := &url.URL{
			Scheme: "http",
			Host:   cfg.Address,
		}
		if cfg.Username != "" && cfg.Password != "" {
			proxyURL.User = url.UserPassword(cfg.Username, cfg.Password)
		}
		h.transport.Proxy = http.ProxyURL(proxyURL)
		h.transport.DialContext = nil // 确保不再使用 SOCKS5 Dialer
	case "socks5":
		var auth *proxy.Auth
		if cfg.Username != "" && cfg.Password != "" {
			auth = &proxy.Auth{
				User:     cfg.Username,
				Password: cfg.Password,
			}
		}
		dialer, err := proxy.SOCKS5("tcp", cfg.Address, auth, proxy.Direct)
		if err != nil {
			return err
		}
		h.transport.Proxy = nil // 确保不再使用 HTTP Proxy
		// 优先使用 ContextDialer，使 SOCKS5 握手阶段能被 context 超时/取消，
		// 避免高并发时 SOCKS5 握手卡死占用并发槽。
		if cd, ok := dialer.(proxy.ContextDialer); ok {
			h.transport.DialContext = cd.DialContext
		} else {
			h.transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
	default:
		return fmt.Errorf("unsupported proxy type: %s", cfg.Type)
	}
	return nil
}

// EnableJA3 开启 JA3 TLS 指纹模拟；profile 为空时等同于 DisableJA3。
// 支持：chrome、firefox、safari、edge、ios。
func (h *HttpClient) EnableJA3(profile string) error {
	if profile == "" {
		h.transport.DialTLSContext = nil
		h.client.Transport = h.transport
		return nil
	}
	h.transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 优先使用已配置的代理 Dialer（如 SOCKS5），避免绕过代理直连
		var rawConn net.Conn
		var err error
		if h.transport.DialContext != nil {
			rawConn, err = h.transport.DialContext(ctx, network, addr)
		} else {
			d := &net.Dialer{Timeout: h.client.Timeout}
			rawConn, err = d.DialContext(ctx, network, addr)
		}
		if err != nil {
			h.LogInfo("DialContext failed", "error", err)
			return nil, err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			rawConn.Close()
			h.LogInfo("SplitHostPort failed", "error", err)
			return nil, fmt.Errorf("invalid addr %s: %v", addr, err)
		}
		clientHelloID := getClientHelloID(profile)
		uConn := utls.UClient(rawConn, &utls.Config{ServerName: host}, clientHelloID)
		// 提前构建 ClientHello，再直接修改 ALPNExtension，
		// 确保只声明 http/1.1，阻止服务端协商 h2。
		// 说明：Config.NextProtos 无法覆盖预设指纹的 ALPN，因为
		// ALPNExtension.writeToUConn 会反向覆盖 config.NextProtos。
		// 正确做法：BuildHandshakeState() 之后找到 ALPNExtension 并修改。
		if err := uConn.BuildHandshakeState(); err != nil {
			rawConn.Close()
			h.LogInfo("BuildHandshakeState failed", "error", err)
			return nil, err
		}
		for _, ext := range uConn.Extensions {
			if alpnExt, ok := ext.(*utls.ALPNExtension); ok {
				alpnExt.AlpnProtocols = []string{"http/1.1"}
				break
			}
		}
		if err := uConn.Handshake(); err != nil {
			rawConn.Close()
			h.LogInfo("TLS handshake failed", "error", err)
			return nil, err
		}
		return uConn, nil
	}
	h.client.Transport = h.transport
	return nil
}

// DisableJA3 关闭 JA3 指纹模拟，恢复默认 TLS。
func (h *HttpClient) DisableJA3() {
	h.LogInfo("DisableJA3 called")
	if h.transport != nil {
		h.transport.DialTLSContext = nil
		h.client.Transport = h.transport
	}
	h.LogInfo("JA3 disabled, using default TLS")
}

// getClientHelloID 将 profile 字符串映射为 utls.ClientHelloID。
func getClientHelloID(profile string) utls.ClientHelloID {
	switch profile {
	case "chrome":
		return utls.HelloChrome_120
	case "firefox":
		return utls.HelloFirefox_102
	case "safari":
		return utls.HelloSafari_16_0
	case "edge":
		return utls.HelloEdge_106
	case "ios":
		return utls.HelloIOS_14
	default:
		return utls.HelloChrome_120
	}
}

// SetTimeout 设置请求超时时间。
func (h *HttpClient) SetTimeout(timeout time.Duration) {
	h.client.Timeout = timeout
	h.transport.IdleConnTimeout = timeout
	h.LogInfo("Timeout set", "duration", timeout)
}

// GetTimeout 返回当前请求超时时间。
func (h *HttpClient) GetTimeout() time.Duration {
	return h.client.Timeout
}

