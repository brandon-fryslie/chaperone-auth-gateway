// Package proxy provides HTTP/HTTPS proxy handlers for MITM and credential injection.
// This file contains the CONNECT handler that decides whether to perform MITM or transparent tunneling.
package proxy

import (
	"crypto/tls"
	"log/slog"

	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/service"
	"github.com/elazarl/goproxy"
)

// connectHandler creates a CONNECT handler that decides whether to MITM or transparently tunnel.
func connectHandler(registry service.ServiceRegistry, certStore *GoproxyCertStore, logger *slog.Logger) func(string, *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	return func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		// Add request ID to context for logging
		reqCtx := log.WithRequestID(ctx.Req.Context())
		ctx.Req = ctx.Req.WithContext(reqCtx)

		// Store request ID and pre-generate correlation color for this connection
		// All requests through this CONNECT tunnel will use the same color
		requestID := log.RequestID(reqCtx)
		r, g, b := log.NextCorrelationColor()
		ctx.UserData = &requestMetadata{
			requestID: requestID,
			colorR:    r,
			colorG:    g,
			colorB:    b,
		}

		if service.ShouldMITM(registry, host) {
			log.Debug(reqCtx, "handling CONNECT request with MITM", "host", host)

			// Create TLS config function that generates certificates per hostname
			tlsConfigFunc := func(host string, ctx *goproxy.ProxyCtx) (*tls.Config, error) {
				cert, err := certStore.Fetch(host, nil)
				if err != nil {
					log.Error(reqCtx, "failed to get certificate for MITM", err, "host", host)
					return nil, err
				}

				log.Debug(reqCtx, "MITM TLS handshake successful", "host", host)
				return &tls.Config{
					Certificates: []tls.Certificate{*cert},
					MinVersion:   tls.VersionTLS12,
				}, nil
			}

			return &goproxy.ConnectAction{
				Action:    goproxy.ConnectMitm,
				TLSConfig: tlsConfigFunc,
			}, host
		}

		log.Debug(reqCtx, "using transparent tunnel for non-configured domain", "host", host)
		return goproxy.OkConnect, host
	}
}
