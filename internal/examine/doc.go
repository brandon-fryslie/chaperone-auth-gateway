// Package examine provides tooling for examining HTTP requests to discover authentication patterns.
//
// The examine package is used by the 'chaperone examine' command to run a passthrough MITM proxy
// that logs requests without modifying them. This helps users understand how credentials are
// actually passed in requests so they can write correct chaperone configuration.
//
// Key features:
// - Header classification: Filters out headers that never contain auth (Content-Type, Accept, etc.)
// - Human-readable logging: Structured output focusing on auth-relevant data
// - Query parameter logging: Auth tokens are sometimes passed in URLs
// - Cookie logging: Session tokens often live in cookies
//
// Example usage:
//
//	logger := examine.NewLogger(examine.Config{})
//	logger.LogRequest(httpRequest)
//
// Output format:
//
//	================================================================================
//	REQUEST: POST https://api.openai.com/v1/chat/completions
//	--------------------------------------------------------------------------------
//	Headers (potentially containing auth):
//	  Authorization: Bearer sk-...
//	  X-Api-Key: abc123...
//
//	Query Parameters:
//	  (none)
//
//	Cookies:
//	  session: xyz...
//	================================================================================
package examine
