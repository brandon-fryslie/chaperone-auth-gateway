package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNarrows_NilTreatedAsWidest(t *testing.T) {
	assert.NoError(t, Narrows(nil, nil))
	assert.NoError(t, Narrows(nil, &Policy{}))
	// nil request against a bounded policy widens on every dimension → reject.
	assert.Error(t, Narrows(nil, &Policy{AllowedMethods: []string{"GET"}}))
}

func TestNarrows_Methods(t *testing.T) {
	cases := []struct {
		name      string
		req, max  []string
		wantError bool
	}{
		{"both all", nil, nil, false},
		{"bound all, req narrows", []string{"GET"}, nil, false},
		{"subset", []string{"GET"}, []string{"GET", "POST"}, false},
		{"equal", []string{"GET", "POST"}, []string{"GET", "POST"}, false},
		{"method outside bound", []string{"DELETE"}, []string{"GET", "POST"}, true},
		{"req all but bound restricts", nil, []string{"GET"}, true},
		{"case-sensitive miss", []string{"get"}, []string{"GET"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Narrows(&Policy{AllowedMethods: c.req}, &Policy{AllowedMethods: c.max})
			assert.Equal(t, c.wantError, err != nil, "err=%v", err)
		})
	}
}

func TestNarrows_Paths(t *testing.T) {
	cases := []struct {
		name      string
		req, max  []string
		wantError bool
	}{
		{"both all", nil, nil, false},
		{"bound all, req narrows", []string{"/v1/*"}, nil, false},
		{"req all but bound restricts", nil, []string{"/v1/*"}, true},
		{"exact within prefix", []string{"/v1/chat"}, []string{"/v1/*"}, false},
		{"prefix equal", []string{"/v1/*"}, []string{"/v1/*"}, false},
		{"sub-prefix within prefix", []string{"/v1/sub/*"}, []string{"/v1/*"}, false},
		{"sibling prefix", []string{"/v2/*"}, []string{"/v1/*"}, true},
		{"prefix without slash not covered", []string{"/v1"}, []string{"/v1/*"}, true},
		{"exact equals exact", []string{"/v1/chat"}, []string{"/v1/chat"}, false},
		{"wildcard not subset of exact", []string{"/v1/*"}, []string{"/v1/chat"}, true},
		{"different exact", []string{"/v1/chats"}, []string{"/v1/chat"}, true},
		{"each req covered by some bound", []string{"/v1/x", "/v2/health"}, []string{"/v1/*", "/v2/health"}, false},
		{"unsupported req glob", []string{"/v1/*/messages"}, []string{"/v1/*"}, true},
		{"unsupported bound glob", []string{"/v1/chat"}, []string{"/v1/*/x"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Narrows(&Policy{AllowedPaths: c.req}, &Policy{AllowedPaths: c.max})
			assert.Equal(t, c.wantError, err != nil, "err=%v", err)
		})
	}
}

func TestNarrows_BodySize(t *testing.T) {
	cases := []struct {
		name      string
		req, max  int64
		wantError bool
	}{
		{"both unlimited", 0, 0, false},
		{"bound unlimited, req limited", 1000, 0, false},
		{"req unlimited but bound limited", 0, 1000, true},
		{"smaller", 500, 1000, false},
		{"equal", 1000, 1000, false},
		{"bigger", 2000, 1000, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Narrows(&Policy{MaxBodyBytes: c.req}, &Policy{MaxBodyBytes: c.max})
			assert.Equal(t, c.wantError, err != nil, "err=%v", err)
		})
	}
}

func TestCanonicalAuthRef(t *testing.T) {
	assert.Equal(t, "bearer", CanonicalAuthRef("bearer", ""))
	assert.Equal(t, "header:X-API-Key", CanonicalAuthRef("header:X-API-Key", ""))
	assert.Equal(t, "header:X-API-Key", CanonicalAuthRef("header", "X-API-Key"))
	// combined and separate forms canonicalize identically
	assert.Equal(t, CanonicalAuthRef("header:X-API-Key", ""), CanonicalAuthRef("header", "X-API-Key"))
}

func TestHeaderNameFromRef(t *testing.T) {
	name, ok := HeaderNameFromRef("header:X-API-Key")
	assert.True(t, ok)
	assert.Equal(t, "X-API-Key", name)

	_, ok = HeaderNameFromRef("bearer")
	assert.False(t, ok)
}
