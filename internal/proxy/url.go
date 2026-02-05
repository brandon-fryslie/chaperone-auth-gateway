package proxy

import (
	"context"
	"net"
	"net/url"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/util"
)

// GetProxyURL returns a properly formatted proxy URL for the given config.
// Always returns a standard http://127.0.0.1:port URL with nil dialer.
//
// Usage with http.Transport:
//
//	proxyURL, dialer := proxy.GetProxyURL(cfg)
//	transport := &http.Transport{
//		Proxy: http.ProxyURL(proxyURL),
//		DialContext: dialer,
//	}
func GetProxyURL(cfg *config.Config) (*url.URL, func(ctx context.Context, network, addr string) (net.Conn, error)) {
	// TCP mode: return standard http:// URL
	host := util.GetProxyURLString(cfg.Server.Address, cfg.Server.Port)
	proxyURL, _ := url.Parse(host)

	return proxyURL, nil
}
