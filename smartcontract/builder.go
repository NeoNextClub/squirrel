package smartcontract

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"math/bits"
	"squirrel/log"
	"strconv"
)

// wordSizeBytes is a size of a big.Word (uint) in bytes.
const wordSizeBytes = bits.UintSize / 8

type scriptBuilder struct {
	b bytes.Buffer
}

// ScriptBuilder is the constructor of smartcontract script
type ScriptBuilder struct {
	sb scriptBuilder

	ScriptHash []byte
	Method     string
	Params     [][]byte
}

func (sb *scriptBuilder) Emit(opCode byte) {
	sb.b.WriteByte(opCode)
}

func (sb *scriptBuilder) EmitPush(number int64) {
	if number == -1 {
		sb.Emit(0x4F)
	} else if number == 0 {
		sb.Emit(0x00)
	} else if number > 0 && number <= 16 {
		sb.Emit(0x51 - 1 + byte(int8(number)))
	} else {
		sb.EmitPushBytes(intToBytes(big.NewInt(number)))
	}
}

func (sb *scriptBuilder) EmitPushBytes(data []byte) {
	length := len(data)
	if length == 0 {
		panic("Can not emit push empty byte slice.")
	}

	if length <= 0x4B {
		sb.b.WriteByte(byte(length))
		sb.b.Write(data)
	} else if length <= 0xFF { // One byte
		sb.Emit(0x4C)
		sb.b.WriteByte(byte(length))
		sb.b.Write(data)
	} else if length <= 0xFFFF { // Two bytes
		sb.Emit(0x4D)
		lengthBytes := make([]byte, 2)
		binary.LittleEndian.PutUint16(lengthBytes, uint16(length))
		sb.b.Write(lengthBytes)
		sb.b.Write(data)
	} else {
		sb.Emit(0x4E)
		lengthBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lengthBytes, uint32(length))
		sb.b.Write(lengthBytes)
		sb.b.Write(data)
	}
}

func (sb *scriptBuilder) EmitAppCall(scriptHash []byte) {
	if len(scriptHash) != 20 {
		panic("Invalid script hash.")
	}

	sb.Emit(0x67)
	sb.b.Write(scriptHash)
}

// Bytes returns the underline bytes.
func (sb *scriptBuilder) Bytes() []byte {
	return sb.b.Bytes()
}

// GetScript returns script string of raw smart contract script
func (scsb *ScriptBuilder) GetScript() string {
	if scsb.Params != nil {
		for i := len(scsb.Params) - 1; i >= 0; i-- {
			scsb.sb.EmitPushBytes(scsb.Params[i])
		}
	}

	scsb.sb.EmitPush(int64(len(scsb.Params)))
	scsb.sb.Emit(0xC1)
	scsb.sb.EmitPushBytes([]byte(scsb.Method))
	scsb.sb.EmitAppCall(scsb.ScriptHash)

	return hex.EncodeToString(scsb.sb.b.Bytes())
}

func intToBytes(n *big.Int) []byte {
	sign := n.Sign()
	if sign == 0 {
		return []byte{0}
	}

	var ws []big.Word
	if sign == 1 {
		ws = n.Bits()
	} else {
		n1 := new(big.Int).Add(n, big.NewInt(1))
		if n1.Sign() == 0 { // n == -1
			return []byte{0xFF}
		}

		ws = n1.Bits()
	}

	data := wordsToBytes(ws)

	size := len(data)
	for ; data[size-1] == 0; size-- {
	}

	data = data[:size]

	if data[size-1]&0x80 != 0 {
		data = append(data, 0)
	}

	if sign == -1 {
		for i := range data {
			data[i] = ^data[i]
		}
	}

	return data
}

func wordsToBytes(ws []big.Word) []byte {
	lb := len(ws) * wordSizeBytes
	bs := make([]byte, lb, lb+1)

	if wordSizeBytes == 8 {
		for i := range ws {
			binary.LittleEndian.PutUint64(bs[i*wordSizeBytes:], uint64(ws[i]))
		}
	} else {
		for i := range ws {
			binary.LittleEndian.PutUint32(bs[i*wordSizeBytes:], uint32(ws[i]))
		}
	}

	return bs
}

// CreateNftPropertiesScript creates scripts for NFT properties RPC call.
func CreateNftPropertiesScript(scriptHash []byte, tokenID string) string {
	tokenIDInteger, err := strconv.ParseInt(tokenID, 10, 64)
	if err != nil {
		log.Error.Println(err)
		return ""
	}

	scsb := ScriptBuilder{
		ScriptHash: scriptHash,
		Method:     "properties",
		Params:     [][]byte{intToBytes(big.NewInt(tokenIDInteger))},
	}

	return scsb.GetScript()
}
