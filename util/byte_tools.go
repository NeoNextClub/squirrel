package util

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// GetValueFromBytes returns integer value of given data bytes.
func GetValueFromBytes(data []byte) int64 {
	switch len(data) {
	case 1:
		return int64(data[0])
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(data)))
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(data)))
	case 8:
		return int64(binary.LittleEndian.Uint64(data))
	default:
		panic(fmt.Errorf("can not get value from data: %#02x", data))
	}
}

// HexToBigInt returns big.Int of given hex string.
func HexToBigInt(hexStr string) *big.Int {
	if len(hexStr) == 0 {
		return big.NewInt(0)
	}

	hexStr = padString(hexStr)
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(fmt.Errorf("failed to decode string to hex: %s", err))
	}
	bytes = ReverseBytes(bytes)

	z := new(big.Int)
	z.SetBytes(bytes)
	return z
}

func padString(str string) string {
	strLen := len(str)
	if strLen >= 16 {
		return str
	}
	if strLen%2 == 0 {
		return str + strings.Repeat("0", 16-strLen)
	}
	return "0" + str + strings.Repeat("0", 16-strLen-1)
}

// BytesToBigFloat returns big.Float of given data bytes.
func BytesToBigFloat(data []byte) *big.Float {
	data = ReverseBytes(data)
	val := new(big.Float).SetInt(new(big.Int).SetBytes(data))
	if val.Sign() == -1 {
		return BytesToBigFloat(append(data, 0x00))
	}
	return val
}

// ReverseBytes reverses the given bytes.
func ReverseBytes(raw []byte) []byte {
	reversed := make([]byte, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		reversed[len(raw)-i-1] = raw[i]
	}
	return reversed
}

// StrToBigFloat returns big.Float of the given integer string.
func StrToBigFloat(str string) *big.Float {
	if len(str) == 0 {
		return big.NewFloat(0)
	}

	val, _, err := big.ParseFloat(str, 10, 256, big.ToZero)
	if err != nil {
		panic(err)
	}
	return val
}
