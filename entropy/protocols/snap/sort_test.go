package snap

import (
	"bytes"
	"fmt"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/database/trie"
	"testing"
)

func hexToNibbles(s string) []byte {
	if len(s) >= 2 && s[0] == '0' && s[1] == 'x' {
		s = s[2:]
	}
	var s2 []byte
	for _, ch := range []byte(s) {
		s2 = append(s2, '0')
		s2 = append(s2, ch)
	}
	return common.Hex2Bytes(string(s2))
}

func TestRequestSorting(t *testing.T) {

	//   - Path 0x9  -> {0x19}
	//   - Path 0x99 -> {0x0099}
	//   - Path 0x01234567890123456789012345678901012345678901234567890123456789019  -> {0x0123456789012345678901234567890101234567890123456789012345678901, 0x19}
	//   - Path 0x012345678901234567890123456789010123456789012345678901234567890199 -> {0x0123456789012345678901234567890101234567890123456789012345678901, 0x0099}
	var f = func(path string) (trie.SyncPath, TrieNodePathSet, common.Hash) {
		data := hexToNibbles(path)
		sp := trie.NewSyncPath(data)
		tnps := TrieNodePathSet([][]byte(sp))
		hash := common.Hash{}
		return sp, tnps, hash
	}
	var (
		hashes   []common.Hash
		paths    []trie.SyncPath
		pathsets []TrieNodePathSet
	)
	for _, x := range []string{
		"0x9",
		"0x012345678901234567890123456789010123456789012345678901234567890195",
		"0x012345678901234567890123456789010123456789012345678901234567890197",
		"0x012345678901234567890123456789010123456789012345678901234567890196",
		"0x99",
		"0x012345678901234567890123456789010123456789012345678901234567890199",
		"0x01234567890123456789012345678901012345678901234567890123456789019",
		"0x0123456789012345678901234567890101234567890123456789012345678901",
		"0x01234567890123456789012345678901012345678901234567890123456789010",
		"0x01234567890123456789012345678901012345678901234567890123456789011",
	} {
		sp, _, hash := f(x)
		hashes = append(hashes, hash)
		paths = append(paths, sp)
	}
	_, paths, pathsets = sortByAccountPath(hashes, paths)
	{
		var b = new(bytes.Buffer)
		for i := 0; i < len(paths); i++ {
			fmt.Fprintf(b, "\n%d. paths %x", i, paths[i])
		}
		want := `
0. paths [0099]
1. paths [0123456789012345678901234567890101234567890123456789012345678901 00]
2. paths [0123456789012345678901234567890101234567890123456789012345678901 0095]
3. paths [0123456789012345678901234567890101234567890123456789012345678901 0096]
4. paths [0123456789012345678901234567890101234567890123456789012345678901 0097]
5. paths [0123456789012345678901234567890101234567890123456789012345678901 0099]
6. paths [0123456789012345678901234567890101234567890123456789012345678901 10]
7. paths [0123456789012345678901234567890101234567890123456789012345678901 11]
8. paths [0123456789012345678901234567890101234567890123456789012345678901 19]
9. paths [19]`
		if have := b.String(); have != want {
			t.Errorf("have:%v\nwant:%v\n", have, want)
		}
	}
	{
		var b = new(bytes.Buffer)
		for i := 0; i < len(pathsets); i++ {
			fmt.Fprintf(b, "\n%d. pathset %x", i, pathsets[i])
		}
		want := `
0. pathset [0099]
1. pathset [0123456789012345678901234567890101234567890123456789012345678901 00 0095 0096 0097 0099 10 11 19]
2. pathset [19]`
		if have := b.String(); have != want {
			t.Errorf("have:%v\nwant:%v\n", have, want)
		}
	}
}