package accounts

import (
	"testing"
)

func TestURLParsing(t *testing.T) {
	url, err := parseURL("https://entropy.org")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if url.Scheme != "https" {
		t.Errorf("expected: %v, got: %v", "https", url.Scheme)
	}
	if url.Path != "entropy.org" {
		t.Errorf("expected: %v, got: %v", "entropy.org", url.Path)
	}

	for _, u := range []string{"entropy.org", ""} {
		if _, err = parseURL(u); err == nil {
			t.Errorf("input %v, expected err, got: nil", u)
		}
	}
}

func TestURLString(t *testing.T) {
	url := URL{Scheme: "https", Path: "entropy.org"}
	if url.String() != "https://entropy.org" {
		t.Errorf("expected: %v, got: %v", "https://entropy.org", url.String())
	}

	url = URL{Scheme: "", Path: "entropy.org"}
	if url.String() != "entropy.org" {
		t.Errorf("expected: %v, got: %v", "entropy.org", url.String())
	}
}

func TestURLMarshalJSON(t *testing.T) {
	url := URL{Scheme: "https", Path: "entropy.org"}
	json, err := url.MarshalJSON()
	if err != nil {
		t.Errorf("unexpcted error: %v", err)
	}
	if string(json) != "\"https://entropy.org\"" {
		t.Errorf("expected: %v, got: %v", "\"https://entropy.org\"", string(json))
	}
}

func TestURLUnmarshalJSON(t *testing.T) {
	url := &URL{}
	err := url.UnmarshalJSON([]byte("\"https://entropy.org\""))
	if err != nil {
		t.Errorf("unexpcted error: %v", err)
	}
	if url.Scheme != "https" {
		t.Errorf("expected: %v, got: %v", "https", url.Scheme)
	}
	if url.Path != "entropy.org" {
		t.Errorf("expected: %v, got: %v", "https", url.Path)
	}
}

func TestURLComparison(t *testing.T) {
	tests := []struct {
		urlA   URL
		urlB   URL
		expect int
	}{
		{URL{"https", "entropy.org"}, URL{"https", "entropy.org"}, 0},
		{URL{"http", "entropy.org"}, URL{"https", "entropy.org"}, -1},
		{URL{"https", "entropy.org/a"}, URL{"https", "entropy.org"}, 1},
		{URL{"https", "abc.org"}, URL{"https", "entropy.org"}, -1},
	}

	for i, tt := range tests {
		result := tt.urlA.Cmp(tt.urlB)
		if result != tt.expect {
			t.Errorf("test %d: cmp mismatch: expected: %d, got: %d", i, tt.expect, result)
		}
	}
}
