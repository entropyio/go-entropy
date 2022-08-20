package trie

import (
	"github.com/entropyio/go-entropy/database/rawdb"
	"testing"
)

// Tests if the trie diffs are tracked correctly.
func TestTrieTracer(t *testing.T) {
	trie := NewEmpty(NewDatabase(rawdb.NewMemoryDatabase()))
	trie.tracer = newTracer()

	// Insert a batch of entries, all the nodes should be marked as inserted
	vals := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"dog", "puppy"},
		{"somethingveryoddindeedthis is", "myothernodedata"},
	}
	for _, val := range vals {
		trie.Update([]byte(val.k), []byte(val.v))
	}
	trie.Hash()

	seen := make(map[string]struct{})
	it := trie.NodeIterator(nil)
	for it.Next(true) {
		if it.Leaf() {
			continue
		}
		seen[string(it.Path())] = struct{}{}
	}
	inserted := trie.tracer.insertList()
	if len(inserted) != len(seen) {
		t.Fatalf("Unexpected inserted node tracked want %d got %d", len(seen), len(inserted))
	}
	for _, k := range inserted {
		_, ok := seen[string(k)]
		if !ok {
			t.Fatalf("Unexpected inserted node")
		}
	}
	deleted := trie.tracer.deleteList()
	if len(deleted) != 0 {
		t.Fatalf("Unexpected deleted node tracked %d", len(deleted))
	}

	// Commit the changes
	trie.Commit(nil)

	// Delete all the elements, check deletion set
	for _, val := range vals {
		trie.Delete([]byte(val.k))
	}
	trie.Hash()

	inserted = trie.tracer.insertList()
	if len(inserted) != 0 {
		t.Fatalf("Unexpected inserted node tracked %d", len(inserted))
	}
	deleted = trie.tracer.deleteList()
	if len(deleted) != len(seen) {
		t.Fatalf("Unexpected deleted node tracked want %d got %d", len(seen), len(deleted))
	}
	for _, k := range deleted {
		_, ok := seen[string(k)]
		if !ok {
			t.Fatalf("Unexpected inserted node")
		}
	}
}

func TestTrieTracerNoop(t *testing.T) {
	trie := NewEmpty(NewDatabase(rawdb.NewMemoryDatabase()))
	trie.tracer = newTracer()

	// Insert a batch of entries, all the nodes should be marked as inserted
	vals := []struct{ k, v string }{
		{"do", "verb"},
		{"ether", "wookiedoo"},
		{"horse", "stallion"},
		{"shaman", "horse"},
		{"doge", "coin"},
		{"dog", "puppy"},
		{"somethingveryoddindeedthis is", "myothernodedata"},
	}
	for _, val := range vals {
		trie.Update([]byte(val.k), []byte(val.v))
	}
	for _, val := range vals {
		trie.Delete([]byte(val.k))
	}
	if len(trie.tracer.insertList()) != 0 {
		t.Fatalf("Unexpected inserted node tracked %d", len(trie.tracer.insertList()))
	}
	if len(trie.tracer.deleteList()) != 0 {
		t.Fatalf("Unexpected deleted node tracked %d", len(trie.tracer.deleteList()))
	}
}
