package util

import (
	"crypto/sha256"
	"encoding/hex"

	//lint:ignore SA1019 used for Neo address script hash calculation.
	"golang.org/x/crypto/ripemd160"
)

// Hash160 returns hash160 of input data bytes.
func Hash160(data []byte) []byte {
	return Ripemd160(Sha256(data))
}

// Hash256 returns hash256 of input data bytes.
func Hash256(data []byte) []byte {
	return Sha256(Sha256(data))
}

// Sha256 returns sha256 of input data bytes.
func Sha256(data []byte) []byte {
	sha256H := sha256.New()
	sha256H.Reset()
	sha256H.Write(data)
	return sha256H.Sum(nil)
}

// Ripemd160 returns RIPEMD-160 hash bytes.
func Ripemd160(data []byte) []byte {
	ripemd160H := ripemd160.New()
	ripemd160H.Reset()
	ripemd160H.Write(data)
	return ripemd160H.Sum(nil)
}

// GetScriptHash returns hash(160 bits) of input data bytes.
func GetScriptHash(data []byte) []byte {
	return Hash160(data)
}

// GetAssetIDFromScriptHash returns assetID from script hash.
func GetAssetIDFromScriptHash(scriptHash []byte) string {
	assetID := hex.EncodeToString(ReverseBytes(scriptHash))
	return assetID
}

// GetScriptHashFromAssetID returns script hash from assetID.
func GetScriptHashFromAssetID(assetID string) []byte {
	scriptHash, err := hex.DecodeString(assetID)
	if err != nil {
		panic(err)
	}

	return ReverseBytes(scriptHash)
}
