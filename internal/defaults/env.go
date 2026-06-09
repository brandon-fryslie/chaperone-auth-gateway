package defaults

// CAEnvVars is the comprehensive list of CA certificate environment variables
// that Chaperone sets for spawned child processes. This ensures TLS/SSL clients
// trust the MITM CA certificate across different tools and languages.
//
// This is the single source of truth for CA environment variables.
// Both run and examine modes use this list.
var CAEnvVars = []string{
	// OpenSSL, cURL, and most CLI tools
	"SSL_CERT_FILE",

	// cURL fallback (some versions check this first)
	"CURL_CA_BUNDLE",

	// Git (doesn't respect SSL_CERT_FILE, needs its own var)
	"GIT_SSL_CAINFO",

	// Python requests library
	"REQUESTS_CA_BUNDLE",

	// Python httpx library
	"HTTPX_CA_BUNDLE",

	// Wget and some Go-based tools
	"HTTPS_CA_FILE",

	// Node.js
	"NODE_EXTRA_CA_CERTS",

	// Perl LWP library
	"PERL_LWP_SSL_CA_FILE",

	// AWS CLI and SDKs
	"AWS_CA_BUNDLE",

	// Homebrew (macOS package manager)
	"HOMEBREW_CERTIFICATE_AUTHORITY",
}
