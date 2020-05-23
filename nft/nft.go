package nft

import (
	"encoding/hex"
	"math/big"
	"squirrel/smartcontract"
	"squirrel/util"
)

// Nft db model.
type Nft struct {
	ID               uint
	AssetID          string
	AdminAddress     string
	Name             string
	Symbol           string
	Decimals         uint8
	TotalSupply      *big.Float
	TxID             string
	BlockIndex       uint
	BlockTime        uint64
	Addresses        uint64
	HoldingAddresses uint64
	Transfers        uint64
}

// NftRegInfo db model.
type NftRegInfo struct {
	TxID          string
	ScriptHash    []byte
	Name          string
	Version       string
	Author        string
	Email         string
	Description   string
	NeedStorage   bool
	ParameterList string
	ReturnType    string
}

//Transaction db model.
type Transaction struct {
	ID         uint
	TxID       string
	AssetID    string
	From       string
	To         string
	Value      *big.Float
	BlockIndex uint
	BlockTime  uint64
}

// Tx represents nft transaction model.
type Tx struct {
	ID    uint
	TxID  string
	From  string
	To    string
	Value *big.Float
}

// GetNftRegInfo extracts op codes from stack,
// and returns nft reg info if stack valid.
func GetNftRegInfo(txID string, opCodeDataStack *smartcontract.DataStack) (*NftRegInfo, bool) {
	if len(*opCodeDataStack) < 9 {
		return nil, false
	}

	for {
		if len(*opCodeDataStack) == 9 {
			break
		}

		opCodeDataStack.PopData()
	}

	scriptBytes := opCodeDataStack.PopData() // Contract Script.
	scriptHash := util.GetScriptHash(scriptBytes)
	// scriptHashHex := util.GetAssetIDFromScriptHash(scriptHash)

	regInfo := NftRegInfo{
		TxID:          txID,
		ScriptHash:    scriptHash,
		ParameterList: hex.EncodeToString(opCodeDataStack.PopData()),
		ReturnType:    hex.EncodeToString(opCodeDataStack.PopData()),
		NeedStorage:   opCodeDataStack.PopData()[0] == 0x01,
		Name:          string(opCodeDataStack.PopData()),
		Version:       string(opCodeDataStack.PopData()),
		Author:        string(opCodeDataStack.PopData()),
		Email:         string(opCodeDataStack.PopData()),
		Description:   string(opCodeDataStack.PopData()),
	}

	return &regInfo, true
}
