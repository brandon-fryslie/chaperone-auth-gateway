package util

import "fmt"

// GetProxyURLString returns the proxy URL as a string suitable for environment variables.
// First argument: socket path OR proxy hostname/address
// Second argument: port (0 means first argument is a socket path)
//
// If port is 0: returns http+unix://<socket_path>
// If port is non-zero: returns http://<host>:<port>
//
// This format is compatible with curl and other tools that understand the http+unix:// scheme.
func GetProxyURLString(socketOrHost string, port int) string {
	if port == 0 {
		// Unix socket mode: use http+unix:// scheme for environment variables
		// This format is understood by curl and similar tools
		return fmt.Sprintf("http+unix://%s", socketOrHost)
	}

	// TCP mode: standard http:// URL
	return fmt.Sprintf("http://%s:%d", socketOrHost, port)
}
