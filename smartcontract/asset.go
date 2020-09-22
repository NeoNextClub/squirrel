package smartcontract

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"squirrel/log"
	"strings"

	"squirrel/asset"
	"squirrel/util"
)

// AssetFingerPrint is used to identify if a script is asset registration script.
const AssetFingerPrint = "68104e656f2e41737365742e437265617465"

// GetAssetInfo parses script content and return asset struct.
func GetAssetInfo(script string) *asset.Asset {
	if !strings.HasSuffix(script, AssetFingerPrint) {
		log.Printf("Can not get asset info from script: %s. Scripts format not match.", script)
		return nil
	}

	dataStack := ReadScript(script)
	if len(*dataStack) == 0 {
		return nil
	}

	dataStack.PopData()

	asset := asset.Asset{
		// BlockIndex
		// Time
		// Version
		// AssetID
		Type:      getAssetType(dataStack.PopData()),
		Name:      getAssetName(dataStack.PopData()),
		Amount:    getAssetAmount(dataStack.PopData()),
		Available: big.NewFloat(0),
		Precision: getAssetPrecision(dataStack.PopData()),
		Owner:     getAssetOwner(dataStack.PopData()),
		Admin:     getAssetAdmin(dataStack.PopData()),
		Issuer:    getAssetIssuer(dataStack.PopData()),
		// Expiration
		Frozen: false,
		// Addresses
		// Transactions
	}

	asset.Amount = new(big.Float).SetPrec(256).Quo(asset.Amount, big.NewFloat(math.Pow10(int(asset.Precision))))

	return &asset
}

func getAssetType(data []byte) string {
	val, err := binary.ReadUvarint(bytes.NewBuffer(data))
	if err != nil {
		panic(fmt.Errorf("can not convert asset type from data: %s: %s", hex.EncodeToString(data), err))
	}

	switch val {
	case 0x40:
		return "CreditFlag"
	case 0x80:
		return "DutyFlag"
	case 0x00:
		return "GoverningToken"
	case 0x01:
		return "UtilityToken"
	case 0x08:
		return "Currency"
	case 0x40 | 0x10:
		return "Share"
	case 0x40 | 0x18:
		return "Invoice"
	case 0x40 | 0x20:
		return "Token"
	default:
		return "Unknown"
	}
}

type assetName []struct {
	Lang string `json:"lang"`
	Name string `json:"name"`
}

func getAssetName(data []byte) string {
	var name assetName
	err := json.Unmarshal([]byte(string(data)), &name)
	if err != nil {
		return hex.EncodeToString(data)
	}

	return name[0].Name
}

func getAssetAmount(data []byte) *big.Float {
	amount := util.BytesToBigFloat(data)
	return amount
}

func getAssetPrecision(data []byte) uint8 {
	return uint8(util.GetValueFromBytes(data))
}

func getAssetOwner(data []byte) string {
	pubKey := hex.EncodeToString(data)
	return pubKey
}

func getAssetAdmin(data []byte) string {
	scriptHash := data
	addr := util.GetAddressFromScriptHash(scriptHash)
	return addr
}

func getAssetIssuer(data []byte) string {
	return getAssetAdmin(data)
}
