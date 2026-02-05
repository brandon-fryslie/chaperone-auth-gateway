package util

import "fmt"

// GetProxyURLString returns the proxy URL as a string suitable for environment variables.
// Always returns http://<host>:<port> format.
func GetProxyURLString(host string, port int) string {
	return fmt.Sprintf("http://%s:%d", host, port)
}
