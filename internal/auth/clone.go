package auth

import (
	"net/http"
)

// CloneRequest creates a deep copy of an HTTP request.
// The returned request has independent URL and headers that can be modified
// without affecting the original request.
//
// Note: The request body is NOT cloned since it may be a stream that cannot
// be rewound. Strategies must not modify the request body.
func CloneRequest(req *http.Request) *http.Request {
	// Use stdlib Clone as base - this copies most fields
	clone := req.Clone(req.Context())

	// Deep copy URL (Clone does shallow copy)
	if req.URL != nil {
		urlCopy := *req.URL
		if req.URL.User != nil {
			// Copy user info if present
			userCopy := *req.URL.User
			urlCopy.User = &userCopy
		}
		clone.URL = &urlCopy
	}

	// Deep copy headers (Clone does shallow copy)
	clone.Header = make(http.Header, len(req.Header))
	for k, v := range req.Header {
		// Copy slice to prevent sharing
		clone.Header[k] = append([]string(nil), v...)
	}

	return clone
}
