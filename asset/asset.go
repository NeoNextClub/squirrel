package asset

import "math/big"

// Constants.
const (
	ASSET = "asset"
	NEP5  = "nep5"
	NFT   = "nft"

	NEO        = "NEO"
	NEOAssetID = "0xc56f33fc6ecfcd0c225c4ab356fee59390af8560be0e930faebe74a6daff7c9b"

	GAS        = "GAS"
	GASAssetID = "0x602c79718b16e442de58778e148d0b1084e3b2dffd5de6b7b16cee7969282de7"
)

// Asset db model.
type Asset struct {
	ID           uint
	BlockIndex   uint
	BlockTime    uint64
	Version      uint
	AssetID      string
	Type         string
	Name         string
	Amount       *big.Float
	Available    *big.Float
	Precision    uint8
	Owner        string
	Admin        string
	Issuer       string
	Expiration   uint64
	Frozen       bool
	Addresses    uint64
	Transactions uint64
}
