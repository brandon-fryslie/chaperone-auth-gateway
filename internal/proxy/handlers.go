// Package proxy provides HTTP/HTTPS proxy handlers for MITM and credential injection.
//
// The proxy package has been refactored into focused files by concern:
//   - connect_handler.go: MITM decision logic for CONNECT requests
//   - policy_handler.go: Policy enforcement (methods, paths, body size, drop patterns)
//   - auth_handler.go: Authentication credential stripping and injection
//   - recording_handler.go: HAR recording for request/response capture
//   - util.go: Shared utilities (request ID, client IP extraction)
//
// All handlers follow a consistent pattern: they are functions that return goproxy handler functions.
// They accept dependencies via parameters (registries, loggers) and use closure to capture state.
package proxy
