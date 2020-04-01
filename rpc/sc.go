package rpc

import (
	"math/big"
)

// SmartContractResponse is the struct of returning data from 'invokescript' rpc call.
type SmartContractResponse struct {
	jsonRPCResponse
	Result *RawSmartContractCallResult `json:"result"`
}

// RawSmartContractCallResult is the inner struct of struct 'SmartContractResponse'.
type RawSmartContractCallResult struct {
	Script      string     `json:"script"`
	State       string     `json:"state"`
	GasConsumed *big.Float `json:"gas_consumed"`
	Stack       []RawStack
}

// RawState is the inner struct of struct 'RawNotifications'.
type RawState struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// GetArray returns RawStack array from RawState.
func (rawState *RawState) GetArray() []RawStack {
	if rawState.Type != "Array" {
		return nil
	}

	arr, ok := rawState.Value.([]interface{})
	if !ok {
		return nil
	}

	rawStacks := []RawStack{}

	for _, state := range arr {
		s := state.(map[string]interface{})
		stack := RawStack{
			Type:  s["type"].(string),
			Value: s["value"],
		}

		rawStacks = append(rawStacks, stack)
	}

	return rawStacks
}

// RawStack is the inner struct of 'RawNotifications'.
type RawStack struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// SmartContractRPCCall returns result of 'invokescript' rpc call.
func SmartContractRPCCall(minHeight int, scripts string) *RawSmartContractCallResult {
	params := []interface{}{scripts}
	args := getRPCRequestBody("invokescript", params)

	respData := SmartContractResponse{}
	rpcCall(minHeight, args, &respData)

	return respData.Result
}
