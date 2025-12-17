// Package shutdown provides coordinated graceful shutdown management.
//
// This package handles SIGTERM/SIGINT signals and coordinates shutdown
// of multiple components in LIFO order with timeout enforcement.
package shutdown
