package proxy

import (
	"testing"

	"github.com/bmf/chaperone/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProxyURL(t *testing.T) {
	t.Run("TCP mode returns http URL", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    4010,
			},
		}

		proxyURL, dialer := GetProxyURL(cfg)

		require.NotNil(t, proxyURL)
		assert.Equal(t, "http", proxyURL.Scheme)
		assert.Equal(t, "127.0.0.1:4010", proxyURL.Host)
		assert.Nil(t, dialer, "TCP mode should not return a custom dialer")
	})
}
