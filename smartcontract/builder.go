package smartcontract

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"squirrel/log"
	"squirrel/util"
	"strconv"
)

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
		bInt := big.NewInt(number)
		val := util.ReverseBytes(bInt.Bytes())
		sb.EmitPushBytes(val)
	}
}

func (sb *scriptBuilder) EmitPushBytes(data []byte) {
	length := len(data)
	if length == 0 {
		panic("Can not emit push empty byte slice.")
	}

	if length == 1 {
		if data[0] == 0x00 ||
			data[0] == 0x4F ||
			(data[0] >= 0x51 && data[0] <= 0x60) {
			sb.b.WriteByte(data[0])
		} else {
			sb.b.WriteByte(0x01)
			sb.b.WriteByte(data[0])
		}
	} else if length <= 0x4B {
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

// CreateNftPropertiesScript creates scripts for NFT properties RPC call.
func CreateNftPropertiesScript(scriptHash []byte, tokenID string) string {
	tokenIDInteger, err := strconv.ParseInt(tokenID, 10, 64)
	if err != nil {
		log.Error.Println(err)
		return ""
	}

	bInt := big.NewInt(tokenIDInteger)
	tokenIDBytes := util.ReverseBytes(bInt.Bytes())

	scsb := ScriptBuilder{
		ScriptHash: scriptHash,
		Method:     "properties",
		Params:     [][]byte{tokenIDBytes},
	}

	return scsb.GetScript()
}
