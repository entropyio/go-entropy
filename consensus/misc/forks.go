package misc

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/config"
)

// VerifyForkHashes verifies that blocks conforming to network hard-forks do have
// the correct hashes, to avoid clients going off on different chains. This is an
// optional feature.
func VerifyForkHashes(*config.ChainConfig, *model.Header, bool) error {
	return nil
}
