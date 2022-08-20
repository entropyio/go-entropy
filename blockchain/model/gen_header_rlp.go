// Code generated by rlpgen. DO NOT EDIT.

//go:build !norlpgen
// +build !norlpgen

package model

import "github.com/entropyio/go-entropy/common/rlp"
import "io"

func (header *Header) EncodeRLP(_w io.Writer) error {
	w := rlp.NewEncoderBuffer(_w)
	_tmp0 := w.List()
	w.WriteBytes(header.ParentHash[:])
	w.WriteBytes(header.UncleHash[:])
	w.WriteBytes(header.Coinbase[:])
	w.WriteBytes(header.Root[:])
	w.WriteBytes(header.TxHash[:])
	w.WriteBytes(header.ReceiptHash[:])
	w.WriteBytes(header.Bloom[:])
	if header.Difficulty == nil {
		w.Write(rlp.EmptyString)
	} else {
		if header.Difficulty.Sign() == -1 {
			return rlp.ErrNegativeBigInt
		}
		w.WriteBigInt(header.Difficulty)
	}
	if header.Number == nil {
		w.Write(rlp.EmptyString)
	} else {
		if header.Number.Sign() == -1 {
			return rlp.ErrNegativeBigInt
		}
		w.WriteBigInt(header.Number)
	}
	w.WriteUint64(header.GasLimit)
	w.WriteUint64(header.GasUsed)
	w.WriteUint64(header.Time)
	w.WriteBytes(header.Extra)
	w.WriteBytes(header.MixDigest[:])
	w.WriteBytes(header.Nonce[:])
	_tmp1 := header.BaseFee != nil
	if _tmp1 {
		if header.BaseFee == nil {
			w.Write(rlp.EmptyString)
		} else {
			if header.BaseFee.Sign() == -1 {
				return rlp.ErrNegativeBigInt
			}
			w.WriteBigInt(header.BaseFee)
		}
	}
	w.ListEnd(_tmp0)
	return w.Flush()
}