// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package genesis

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/common/mathutil"
	"github.com/entropyio/go-entropy/config"
)

var _ = (*genesisSpecMarshaling)(nil)

func (g Genesis) MarshalJSON() ([]byte, error) {
	type Genesis struct {
		Config     *config.ChainConfig                         `json:"config"`
		Nonce      mathutil.HexOrDecimal64                     `json:"nonce"`
		Timestamp  mathutil.HexOrDecimal64                     `json:"timestamp"`
		ExtraData  hexutil.Bytes                               `json:"extraData"`
		GasLimit   mathutil.HexOrDecimal64                     `json:"gasLimit"   gencodec:"required"`
		Difficulty *mathutil.HexOrDecimal256                   `json:"difficulty" gencodec:"required"`
		Mixhash    common.Hash                                 `json:"mixHash"`
		Coinbase   common.Address                              `json:"coinbase"`
		Alloc      map[common.UnprefixedAddress]GenesisAccount `json:"alloc"      gencodec:"required"`
		Number     mathutil.HexOrDecimal64                     `json:"number"`
		GasUsed    mathutil.HexOrDecimal64                     `json:"gasUsed"`
		ParentHash common.Hash                                 `json:"parentHash"`
	}
	var enc Genesis
	enc.Config = g.Config
	enc.Nonce = mathutil.HexOrDecimal64(g.Nonce)
	enc.Timestamp = mathutil.HexOrDecimal64(g.Timestamp)
	enc.ExtraData = g.ExtraData
	enc.GasLimit = mathutil.HexOrDecimal64(g.GasLimit)
	enc.Difficulty = (*mathutil.HexOrDecimal256)(g.Difficulty)
	enc.Mixhash = g.Mixhash
	enc.Coinbase = g.Coinbase
	if g.Alloc != nil {
		enc.Alloc = make(map[common.UnprefixedAddress]GenesisAccount, len(g.Alloc))
		for k, v := range g.Alloc {
			enc.Alloc[common.UnprefixedAddress(k)] = v
		}
	}
	enc.Number = mathutil.HexOrDecimal64(g.Number)
	enc.GasUsed = mathutil.HexOrDecimal64(g.GasUsed)
	enc.ParentHash = g.ParentHash
	return json.Marshal(&enc)
}

func (g *Genesis) UnmarshalJSON(input []byte) error {
	type Genesis struct {
		Config     *config.ChainConfig                         `json:"config"`
		Nonce      *mathutil.HexOrDecimal64                    `json:"nonce"`
		Timestamp  *mathutil.HexOrDecimal64                    `json:"timestamp"`
		ExtraData  *hexutil.Bytes                              `json:"extraData"`
		GasLimit   *mathutil.HexOrDecimal64                    `json:"gasLimit"   gencodec:"required"`
		Difficulty *mathutil.HexOrDecimal256                   `json:"difficulty" gencodec:"required"`
		Mixhash    *common.Hash                                `json:"mixHash"`
		Coinbase   *common.Address                             `json:"coinbase"`
		Alloc      map[common.UnprefixedAddress]GenesisAccount `json:"alloc"      gencodec:"required"`
		Number     *mathutil.HexOrDecimal64                    `json:"number"`
		GasUsed    *mathutil.HexOrDecimal64                    `json:"gasUsed"`
		ParentHash *common.Hash                                `json:"parentHash"`
	}
	var dec Genesis
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Config != nil {
		g.Config = dec.Config
	}
	if dec.Nonce != nil {
		g.Nonce = uint64(*dec.Nonce)
	}
	if dec.Timestamp != nil {
		g.Timestamp = uint64(*dec.Timestamp)
	}
	if dec.ExtraData != nil {
		g.ExtraData = *dec.ExtraData
	}
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Genesis")
	}
	g.GasLimit = uint64(*dec.GasLimit)
	if dec.Difficulty == nil {
		return errors.New("missing required field 'difficulty' for Genesis")
	}
	g.Difficulty = (*big.Int)(dec.Difficulty)
	if dec.Mixhash != nil {
		g.Mixhash = *dec.Mixhash
	}
	if dec.Coinbase != nil {
		g.Coinbase = *dec.Coinbase
	}
	if dec.Alloc == nil {
		return errors.New("missing required field 'alloc' for Genesis")
	}
	g.Alloc = make(GenesisAlloc, len(dec.Alloc))
	for k, v := range dec.Alloc {
		g.Alloc[common.Address(k)] = v
	}
	if dec.Number != nil {
		g.Number = uint64(*dec.Number)
	}
	if dec.GasUsed != nil {
		g.GasUsed = uint64(*dec.GasUsed)
	}
	if dec.ParentHash != nil {
		g.ParentHash = *dec.ParentHash
	}
	return nil
}
