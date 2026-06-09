// Package grant defines the human-owned universe of grantable credential↔host
// pairings and the single enforcer that decides whether a proposed grant is
// authorized.
//
// A GRANT is a *service.Service that Claude (via the MCP control API) asks the
// daemon to activate at runtime. The trust boundary is PRE-APPROVED PAIRINGS:
// the human declares once, in config, which (credential_ref ↔ host_pattern ↔
// auth_strategy) bindings exist and the widest policy a grant against each may
// request. The Enforcer is the [LAW:single-enforcer] for "what is grantable" —
// the control API and MCP server defer to it and never re-decide.
//
// # Accept / reject table (the enumeration-gap spec)
//
// A proposed service is authorized IFF its IDENTITY matches exactly one approved
// pairing AND its SCOPE narrows within that pairing's maximal bound AND it sets
// no operator-only field. Every row below is mirrored by a unit test.
//
// IDENTITY — must match an approved pairing EXACTLY (after canonicalization):
//
//	dimension        comparison
//	credential_ref   exact string equality (no prefix/substring)
//	host_pattern     equality after ToLower+TrimSpace (registry normalization)
//	auth_strategy    equality after CanonicalAuthRef folds header:NAME / header+header_name
//
//	If no pairing matches all three → REJECT "no approved pairing".
//
// SCOPE — requested policy must NARROW within the pairing's maximal bound:
//
//	AllowedMethods (empty = all methods = widest)
//	  bound []      → any request OK (bound unrestricted)
//	  bound [G,P]   → request ⊆ {G,P} OK;  request [] (all) REJECT;  request with M∉bound REJECT
//	  comparison is case-sensitive (matches the proxy's CheckMethod)
//
//	AllowedPaths (empty = all paths = widest; patterns: exact or "<prefix>/*")
//	  bound []        → any request OK
//	  bound non-empty → request [] (all) REJECT; else every requested pattern must be
//	                    contained by some bound pattern (containment defined below)
//	  unsupported glob (anything other than exact or trailing "/*") → REJECT (cannot prove)
//
//	MaxBodyBytes (0 = unlimited = widest)
//	  bound 0   → any request OK (including 0)
//	  bound B>0 → request 0 REJECT (unlimited > B); request r, 0<r≤B OK; request r>B REJECT
//
// OPERATOR-ONLY — must be empty in a runtime grant (inverted monotonicity; not
// Claude's to set). Any non-empty value → REJECT:
//
//	ClientGroups, Drop, Strip
//
// IGNORED by the enforcer:
//
//	Name        registry key, not a trust-boundary concern
//	Placeholder a client-presented gate; can only ADD a precondition to injection,
//	            never widen which credential reaches which host
//
// # Path-pattern containment (outer ⊇ inner)
//
// Patterns are exact ("/v1/chat") or wildcard ("<prefix>/*", e.g. "/v1/*" → prefix "/v1/").
// inner is contained by outer when every path matching inner also matches outer:
//
//	outer exact    : inner exact and equal
//	outer "<p>/*"  : inner exact e with HasPrefix(e, "<p>/"), OR inner "<q>/*" with HasPrefix("<q>/", "<p>/")
//
// This is SOUND (never accepts a widening) and complete for the supported vocabulary;
// anything outside it is rejected rather than guessed.
package grant
