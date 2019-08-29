package rpc

import "testing"

func TestWSGetConfigNoAuth(t *testing.T) {
	config, err := wsGetConfig("ws://example.com:1234", "")
	if err != nil {
		t.Fatalf("wsGetConfig failed: %s", err)
	}
	if config.Location.User != nil {
		t.Fatalf("User should have been stripped from the URL")
	}
	if config.Location.Hostname() != "example.com" ||
		config.Location.Port() != "1234" || config.Location.Scheme != "ws" {
		t.Fatalf("Unexpected URL: %s", config.Location)
	}
}

func TestWSGetConfigWithBasicAuth(t *testing.T) {
	config, err := wsGetConfig("wss://testuser:test-PASS_01@example.com:1234", "")
	if err != nil {
		t.Fatalf("wsGetConfig failed: %s", err)
	}
	if config.Location.User != nil {
		t.Fatal("User should have been stripped from the URL")
	}
	if config.Header.Get("Authorization") != "Basic dGVzdHVzZXI6dGVzdC1QQVNTXzAx" {
		t.Fatal("Basic auth header is incorrect")
	}
}
