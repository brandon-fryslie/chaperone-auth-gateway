package proxy

import (
	"net/http"

	"github.com/bmf/chaperone/internal/service"
	"github.com/elazarl/goproxy"
)

// ChaperoneCondition returns a goproxy condition that matches requests
// for hosts that should be MITM'd according to the service registry.
func ChaperoneCondition(registry service.ServiceRegistry) goproxy.ReqConditionFunc {
	return func(req *http.Request, ctx *goproxy.ProxyCtx) bool {
		return service.ShouldMITM(registry, req.Host)
	}
}

// ChaperoneRespCondition returns a goproxy condition that matches responses
// for hosts that should be MITM'd according to the service registry.
func ChaperoneRespCondition(registry service.ServiceRegistry) goproxy.RespConditionFunc {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) bool {
		if resp == nil || resp.Request == nil {
			return false
		}
		return service.ShouldMITM(registry, resp.Request.Host)
	}
}
