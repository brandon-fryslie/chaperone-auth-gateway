package proxy

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/util"
)

// GetProxyURL returns a properly formatted proxy URL and dialer function for the given config.
// For Unix sockets, it returns a URL with unix:// scheme and a custom dialer.
// For TCP mode, it returns a standard http:// URL and nil dialer.
//
// Usage with http.Transport:
//
//	proxyURL, dialer := proxy.GetProxyURL(cfg)
//	transport := &http.Transport{
//		Proxy: http.ProxyURL(proxyURL),
//		DialContext: dialer,
//	}
func GetProxyURL(cfg *config.Config) (*url.URL, func(ctx context.Context, network, addr string) (net.Conn, error)) {
	if cfg.Server.Socket != "" {
		// Unix socket mode: return unix:// URL and custom dialer
		proxyURL := &url.URL{
			Scheme: "http",
			Host:   "unix",
		}

		// Custom dialer that connects to the Unix socket
		dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
			// For proxy requests, connect to Unix socket instead
			if strings.HasPrefix(addr, "unix:") || addr == "unix" {
				var d net.Dialer
				return d.DialContext(ctx, "unix", cfg.Server.Socket)
			}
			// For non-proxy connections, use default dialer
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}

		return proxyURL, dialer
	}

	// TCP mode: return standard http:// URL
	host := util.GetProxyURLString(cfg.Server.Address, cfg.Server.Port)
	proxyURL, _ := url.Parse(host)

	return proxyURL, nil
}
