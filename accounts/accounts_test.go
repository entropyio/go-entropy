package accounts

import (
	"bytes"
	"testing"

	"github.com/entropyio/go-entropy/common/hexutil"
)

func TestTextHash(t *testing.T) {
	hash := TextHash([]byte("Hello Joe"))
	want := hexutil.MustDecode("0x5a93a91fb560469e532289ca36bd94e8d62d3896b63a790865a71584f203b57a")
	if !bytes.Equal(hash, want) {
		t.Fatalf("wrong hash: %x", hash)
	}
}
