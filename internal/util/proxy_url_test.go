package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProxyURLString(t *testing.T) {
	t.Run("TCP mode with host and port", func(t *testing.T) {
		urlStr := GetProxyURLString("127.0.0.1", 4010)
		assert.Equal(t, "http://127.0.0.1:4010", urlStr)
	})

	t.Run("Different host and port", func(t *testing.T) {
		urlStr := GetProxyURLString("localhost", 8080)
		assert.Equal(t, "http://localhost:8080", urlStr)
	})

	t.Run("IPv6 address", func(t *testing.T) {
		urlStr := GetProxyURLString("::1", 4010)
		assert.Equal(t, "http://::1:4010", urlStr)
	})
}
