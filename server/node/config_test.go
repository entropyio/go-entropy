package node

import (
	"bytes"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/server/p2p"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Tests that datadirs can be successfully created, be them manually configured
// ones or automatically generated temporary ones.
func TestDatadirCreation(t *testing.T) {
	// Create a temporary data dir and check that it can be used by a node
	dir := t.TempDir()

	node, err := New(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("failed to create stack with existing datadir: %v", err)
	}
	if err := node.Close(); err != nil {
		t.Fatalf("failed to close node: %v", err)
	}
	// Generate a long non-existing datadir path and check that it gets created by a node
	dir = filepath.Join(dir, "a", "b", "c", "d", "e", "f")
	node, err = New(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("failed to create stack with creatable datadir: %v", err)
	}
	if err := node.Close(); err != nil {
		t.Fatalf("failed to close node: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("freshly created datadir not accessible: %v", err)
	}
	// Verify that an impossible datadir fails creation
	file, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	defer func() {
		file.Close()
		_ = os.Remove(file.Name())
	}()

	dir = filepath.Join(file.Name(), "invalid/path")
	_, err = New(&Config{DataDir: dir})
	if err == nil {
		t.Fatalf("protocol stack created with an invalid datadir")
	}
}

// Tests that IPC paths are correctly resolved to valid endpoints of different
// platforms.
func TestIPCPathResolution(t *testing.T) {
	var tests = []struct {
		DataDir  string
		IPCPath  string
		Windows  bool
		Endpoint string
	}{
		{"", "", false, ""},
		{"data", "", false, ""},
		{"", "entropy.ipc", false, filepath.Join(os.TempDir(), "entropy.ipc")},
		{"data", "entropy.ipc", false, "data/entropy.ipc"},
		{"data", "./entropy.ipc", false, "./entropy.ipc"},
		{"data", "/entropy.ipc", false, "/entropy.ipc"},
		{"", "", true, ``},
		{"data", "", true, ``},
		{"", "entropy.ipc", true, `\\.\pipe\entropy.ipc`},
		{"data", "entropy.ipc", true, `\\.\pipe\entropy.ipc`},
		{"data", `\\.\pipe\entropy.ipc`, true, `\\.\pipe\entropy.ipc`},
	}
	for i, test := range tests {
		// Only run when platform/test match
		if (runtime.GOOS == "windows") == test.Windows {
			if endpoint := (&Config{DataDir: test.DataDir, IPCPath: test.IPCPath}).IPCEndpoint(); endpoint != test.Endpoint {
				t.Errorf("test %d: IPC endpoint mismatch: have %s, want %s", i, endpoint, test.Endpoint)
			}
		}
	}
}

// Tests that node keys can be correctly created, persisted, loaded and/or made
// ephemeral.
func TestNodeKeyPersistency(t *testing.T) {
	// Create a temporary folder and make sure no key is present
	dir := t.TempDir()

	keyfile := filepath.Join(dir, "unit-test", datadirPrivateKey)

	// Configure a node with a preset key and ensure it's not persisted
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate one-shot node key: %v", err)
	}
	config := &Config{Name: "unit-test", DataDir: dir, P2P: p2p.Config{PrivateKey: key}}
	config.NodeKey()
	if _, err := os.Stat(filepath.Join(keyfile)); err == nil {
		t.Fatalf("one-shot node key persisted to data directory")
	}

	// Configure a node with no preset key and ensure it is persisted this time
	config = &Config{Name: "unit-test", DataDir: dir}
	config.NodeKey()
	if _, err := os.Stat(keyfile); err != nil {
		t.Fatalf("node key not persisted to data directory: %v", err)
	}
	if _, err = crypto.LoadECDSA(keyfile); err != nil {
		t.Fatalf("failed to load freshly persisted node key: %v", err)
	}
	blob1, err := os.ReadFile(keyfile)
	if err != nil {
		t.Fatalf("failed to read freshly persisted node key: %v", err)
	}

	// Configure a new node and ensure the previously persisted key is loaded
	config = &Config{Name: "unit-test", DataDir: dir}
	config.NodeKey()
	blob2, err := os.ReadFile(filepath.Join(keyfile))
	if err != nil {
		t.Fatalf("failed to read previously persisted node key: %v", err)
	}
	if !bytes.Equal(blob1, blob2) {
		t.Fatalf("persisted node key mismatch: have %x, want %x", blob2, blob1)
	}

	// Configure ephemeral node and ensure no key is dumped locally
	config = &Config{Name: "unit-test", DataDir: ""}
	config.NodeKey()
	if _, err := os.Stat(filepath.Join(".", "unit-test", datadirPrivateKey)); err == nil {
		t.Fatalf("ephemeral node key persisted to disk")
	}
}
