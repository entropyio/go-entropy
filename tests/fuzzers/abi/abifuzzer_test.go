package abi

import (
	"testing"
)

// TestReplicate can be used to replicate crashers from the fuzzing tests.
// Just replace testString with the data in .quoted
func TestReplicate(t *testing.T) {
	testString := "\x20\x20\x20\x20\x20\x20\x20\x20\x80\x00\x00\x00\x20\x20\x20\x20\x00"
	data := []byte(testString)
	runFuzzer(data)
}

// TestGenerateCorpus can be used to add corpus for the fuzzer.
// Just replace corpusHex with the hexEncoded output you want to add to the fuzzer.
func TestGenerateCorpus(t *testing.T) {
	/*
		corpusHex := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		data := common.FromHex(corpusHex)
		checksum := sha1.Sum(data)
		outf := fmt.Sprintf("corpus/%x", checksum)
		if err := os.WriteFile(outf, data, 0777); err != nil {
			panic(err)
		}
	*/
}
