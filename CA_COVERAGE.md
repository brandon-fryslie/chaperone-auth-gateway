# CA Certificate Environment Variables

Chaperone sets comprehensive CA certificate environment variables to ensure all tools and libraries trust the MITM certificate.

## Variables Set

| Environment Variable | Tools/Libraries |
|---------------------|-----------------|
| `SSL_CERT_FILE` | OpenSSL, LibSSL, Go stdlib, many CLI tools |
| `CURL_CA_BUNDLE` | cURL, libcurl-based tools |
| `NODE_EXTRA_CA_CERTS` | Node.js runtime |
| `REQUESTS_CA_BUNDLE` | Python `requests` library |
| `HTTPX_CA_BUNDLE` | Python `httpx` library |
| `GIT_SSL_CAINFO` | Git |
| `PERL_LWP_SSL_CA_FILE` | Perl LWP (libwww-perl) |
| `HTTPS_CA_FILE` | Perl HTTPS modules |
| `AWS_CA_BUNDLE` | AWS CLI, boto3 |
| `HOMEBREW_CERTIFICATE_AUTHORITY` | Homebrew (macOS) |
| `CHAPERONE_CA_CERT` | Custom Chaperone variable for inspection |

## Coverage

These variables cover:
- **Languages:** Go, Node.js, Python, Perl, Ruby (via OpenSSL)
- **Tools:** curl, wget, git, AWS CLI, Homebrew
- **Libraries:** OpenSSL, requests, httpx, LWP, boto3

## Known Gaps

**Java:** Does not use environment variables. Requires system property:
```bash
-Djavax.net.ssl.trustStore=/path/to/truststore
```

**Custom Applications:** Some applications use hardcoded cert stores or ignore env vars. These may require additional configuration.

## Implementation

See `internal/run/env.go` - `SetCAEnvVars()` function.
