package blockchain

import "github.com/entropyio/go-entropy/common"

// BadHashes represent a set of manually tracked bad hashes (usually hard forks)
var BadHashes = map[common.Hash]bool{
	//common.HexToHash("05bef30ef572270f654746da22639a7a0c97dd97a7050b9e252391996aaeb689"): true,
}
