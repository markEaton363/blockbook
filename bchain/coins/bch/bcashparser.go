package bch

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"fmt"

	"github.com/jakm/bchutil"
	"github.com/jakm/btcutil"
	"github.com/jakm/btcutil/chaincfg"
	"github.com/jakm/btcutil/txscript"
	"github.com/schancel/cashaddr-converter/address"
)

type AddressFormat = uint8

const (
	Legacy AddressFormat = iota
	CashAddr
)

const (
	MainNetPrefix = "bitcoincash:"
	TestNetPrefix = "bchtest:"
	RegTestPrefix = "bchreg:"
)

// BCashParser handle
type BCashParser struct {
	*btc.BitcoinParser
	AddressFormat AddressFormat
}

// NewBCashParser returns new BCashParser instance
func NewBCashParser(params *chaincfg.Params, c *btc.Configuration) (*BCashParser, error) {
	var format AddressFormat
	switch c.AddressFormat {
	case "":
		fallthrough
	case "cashaddr":
		format = CashAddr
	case "legacy":
		format = Legacy
	default:
		return nil, fmt.Errorf("Unknown address format: %s", c.AddressFormat)
	}
	p := &BCashParser{
		BitcoinParser: &btc.BitcoinParser{
			BaseParser: &bchain.BaseParser{
				BlockAddressesToKeep: c.BlockAddressesToKeep,
				AmountDecimalPoint:   8,
			},
			Params: params,
		},
		AddressFormat: format,
	}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p, nil
}

// GetChainParams contains network parameters for the main Bitcoin Cash network,
// the regression test Bitcoin Cash network, the test Bitcoin Cash network and
// the simulation test Bitcoin Cash network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	var params *chaincfg.Params
	switch chain {
	case "test":
		params = &chaincfg.TestNet3Params
		params.Net = bchutil.TestnetMagic
	case "regtest":
		params = &chaincfg.RegressionNetParams
		params.Net = bchutil.Regtestmagic
	default:
		params = &chaincfg.MainNetParams
		params.Net = bchutil.MainnetMagic
	}

	return params
}

// GetAddrDescFromAddress returns internal address representation of given address
func (p *BCashParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	return p.addressToOutputScript(address)
}

// addressToOutputScript converts bitcoin address to ScriptPubKey
func (p *BCashParser) addressToOutputScript(address string) ([]byte, error) {
	if isCashAddr(address) {
		da, err := bchutil.DecodeAddress(address, p.Params)
		if err != nil {
			return nil, err
		}
		script, err := bchutil.PayToAddrScript(da)
		if err != nil {
			return nil, err
		}
		return script, nil
	} else {
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
}

func isCashAddr(addr string) bool {
	n := len(addr)
	switch {
	case n > len(MainNetPrefix) && addr[0:len(MainNetPrefix)] == MainNetPrefix:
		return true
	case n > len(TestNetPrefix) && addr[0:len(TestNetPrefix)] == TestNetPrefix:
		return true
	case n > len(RegTestPrefix) && addr[0:len(RegTestPrefix)] == RegTestPrefix:
		return true
	}

	return false
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *BCashParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	a, err := bchutil.ExtractPkScriptAddrs(script, p.Params)
	if err != nil {
		return nil, false, err
	}
	// EncodeAddress returns CashAddr address
	addr := a.EncodeAddress()
	if p.AddressFormat == Legacy {
		da, err := address.NewFromString(addr)
		if err != nil {
			return nil, false, err
		}
		ca, err := da.Legacy()
		if err != nil {
			return nil, false, err
		}
		addr, err = ca.Encode()
		if err != nil {
			return nil, false, err
		}
	}
	return []string{addr}, len(addr) > 0, nil
}
