package config

// These are network parameters that need to be constant between clients, but
// aren't necessarily consensus related.

const (
	// BloomBitsBlocks is the number of blocks a single bloom bit section vector
	// contains.
	BloomBitsBlocks uint64 = 4096
)
