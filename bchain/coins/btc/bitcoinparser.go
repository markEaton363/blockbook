package btc

import (
	"blockbook/bchain"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/big"

	vlq "github.com/bsm/go-vlq"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/wire"
	"github.com/jakm/btcutil"
	"github.com/jakm/btcutil/chaincfg"
	"github.com/jakm/btcutil/txscript"
)

// OutputScriptToAddressesFunc converts ScriptPubKey to bitcoin addresses
type OutputScriptToAddressesFunc func(script []byte) ([]string, bool, error)

// BitcoinParser handle
type BitcoinParser struct {
	*bchain.BaseParser
	Params                      *chaincfg.Params
	OutputScriptToAddressesFunc OutputScriptToAddressesFunc
}

// NewBitcoinParser returns new BitcoinParser instance
func NewBitcoinParser(params *chaincfg.Params, c *Configuration) *BitcoinParser {
	p := &BitcoinParser{
		BaseParser: &bchain.BaseParser{
			BlockAddressesToKeep: c.BlockAddressesToKeep,
			AmountDecimalPoint:   8,
		},
		Params: params,
	}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
}

// GetChainParams contains network parameters for the main Bitcoin network,
// the regression test Bitcoin network, the test Bitcoin network and
// the simulation test Bitcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	}
	return &chaincfg.MainNetParams
}

// GetAddrDescFromVout returns internal address representation (descriptor) of given transaction output
func (p *BitcoinParser) GetAddrDescFromVout(output *bchain.Vout) (bchain.AddressDescriptor, error) {
	return hex.DecodeString(output.ScriptPubKey.Hex)
}

// GetAddrDescFromAddress returns internal address representation (descriptor) of given address
func (p *BitcoinParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	return p.addressToOutputScript(address)
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *BitcoinParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	return p.OutputScriptToAddressesFunc(addrDesc)
}

// GetScriptFromAddrDesc returns output script for given address descriptor
func (p *BitcoinParser) GetScriptFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]byte, error) {
	return addrDesc, nil
}

// addressToOutputScript converts bitcoin address to ScriptPubKey
func (p *BitcoinParser) addressToOutputScript(address string) ([]byte, error) {
	da, err := btcutil.DecodeAddress(address, p.Params)
	if err != nil {
		return nil, err
	}
	script, err := txscript.PayToAddrScript(da)
	if err != nil {
		return nil, err
	}
	return script, nil
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *BitcoinParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	sc, addresses, _, err := txscript.ExtractPkScriptAddrs(script, p.Params)
	if err != nil {
		return nil, false, err
	}
	rv := make([]string, len(addresses))
	for i, a := range addresses {
		rv[i] = a.EncodeAddress()
	}
	var s bool
	if sc != txscript.NonStandardTy && sc != txscript.NullDataTy {
		s = true
	} else {
		if len(script) > 1 && script[0] == txscript.OP_RETURN && len(rv) == 0 {
			l := int(script[1])
			data := script[2:]
			if l == len(data) {
				isASCII := true
				for _, c := range data {
					if c < 32 || c > 127 {
						isASCII = false
						break
					}
				}
				var ed string
				if isASCII {
					ed = "(" + string(data) + ")"
				} else {
					ed = hex.EncodeToString([]byte{byte(l)}) + " " + hex.EncodeToString(data)
				}
				rv = []string{"OP_RETURN " + ed}
			}
		}
	}
	return rv, s, nil
}

func (p *BitcoinParser) TxFromMsgTx(t *wire.MsgTx, parseAddresses bool) bchain.Tx {
	vin := make([]bchain.Vin, len(t.TxIn))
	for i, in := range t.TxIn {
		if blockchain.IsCoinBaseTx(t) {
			vin[i] = bchain.Vin{
				Coinbase: hex.EncodeToString(in.SignatureScript),
				Sequence: in.Sequence,
			}
			break
		}
		s := bchain.ScriptSig{
			Hex: hex.EncodeToString(in.SignatureScript),
			// missing: Asm,
		}
		vin[i] = bchain.Vin{
			Txid:      in.PreviousOutPoint.Hash.String(),
			Vout:      in.PreviousOutPoint.Index,
			Sequence:  in.Sequence,
			ScriptSig: s,
		}
	}
	vout := make([]bchain.Vout, len(t.TxOut))
	for i, out := range t.TxOut {
		addrs := []string{}
		if parseAddresses {
			addrs, _, _ = p.OutputScriptToAddressesFunc(out.PkScript)
		}
		s := bchain.ScriptPubKey{
			Hex:       hex.EncodeToString(out.PkScript),
			Addresses: addrs,
			// missing: Asm,
			// missing: Type,
		}
		var vs big.Int
		vs.SetInt64(out.Value)
		vout[i] = bchain.Vout{
			ValueSat:     vs,
			N:            uint32(i),
			ScriptPubKey: s,
		}
	}
	tx := bchain.Tx{
		Txid:     t.TxHash().String(),
		Version:  t.Version,
		LockTime: t.LockTime,
		Vin:      vin,
		Vout:     vout,
		// skip: BlockHash,
		// skip: Confirmations,
		// skip: Time,
		// skip: Blocktime,
	}
	return tx
}

// ParseTx parses byte array containing transaction and returns Tx struct
func (p *BitcoinParser) ParseTx(b []byte) (*bchain.Tx, error) {
	t := wire.MsgTx{}
	r := bytes.NewReader(b)
	if err := t.Deserialize(r); err != nil {
		return nil, err
	}
	tx := p.TxFromMsgTx(&t, true)
	tx.Hex = hex.EncodeToString(b)
	return &tx, nil
}

// ParseBlock parses raw block to our Block struct
func (p *BitcoinParser) ParseBlock(b []byte) (*bchain.Block, error) {
	w := wire.MsgBlock{}
	r := bytes.NewReader(b)

	if err := w.Deserialize(r); err != nil {
		return nil, err
	}

	txs := make([]bchain.Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.TxFromMsgTx(t, false)
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: w.Header.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}

// PackTx packs transaction to byte array
func (p *BitcoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	buf := make([]byte, 4+vlq.MaxLen64+len(tx.Hex)/2)
	binary.BigEndian.PutUint32(buf[0:4], height)
	vl := vlq.PutInt(buf[4:4+vlq.MaxLen64], blockTime)
	hl, err := hex.Decode(buf[4+vl:], []byte(tx.Hex))
	return buf[0 : 4+vl+hl], err
}

// UnpackTx unpacks transaction from byte array
func (p *BitcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	height := binary.BigEndian.Uint32(buf)
	bt, l := vlq.Int(buf[4:])
	tx, err := p.ParseTx(buf[4+l:])
	if err != nil {
		return nil, 0, err
	}
	tx.Blocktime = bt

	return tx, height, nil
}
