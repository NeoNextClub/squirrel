/*
To restart this task from beginning, execute the following sqls:

TRUNCATE TABLE `addr_asset_nft`;
DELETE FROM `addr_tx` WHERE `asset_type` = 'nft';
UPDATE `address` SET `trans_nft` = 0 WHERE 1=1;
UPDATE `counter` SET
	`last_tx_pk_for_nft` = 0,
	`nft_app_log_idx` = -1,
	`nft_tx_pk_for_addr_tx`=0
WHERE `id` = 1;
TRUNCATE TABLE `nft`;
TRUNCATE TABLE `nft_reg_info`;
TRUNCATE TABLE `nft_tx`;
TRUNCATE TABLE `nft_token`;

*/

package tasks

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"squirrel/cache"
	"squirrel/log"
	"squirrel/mail"
	"squirrel/nft"
	"squirrel/smartcontract"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"squirrel/db"
	"squirrel/rpc"
	"squirrel/tx"
	"squirrel/util"
)

const nftChanSize = 5000

var (
	// NftMaxPkShouldRefresh indicates if highest pk should be refreshed
	nftMaxPkShouldRefresh bool

	// Cache decimals of nft asset
	nftAssetDecimals map[string]uint8
	nftProgress      = Progress{}
	maxNftPK         uint

	// nftAppLogs stores txid with its applicationlog rpc response
	nftAppLogs sync.Map
)

type nftTxInfo struct {
	tx           *tx.Transaction
	dataStack    *smartcontract.DataStack
	appLogResult *rpc.RawApplicationLogResult
}

type nftStore struct {
	// 0: nft reg
	// 1: nft tx
	// 2: nft addr balance and total supply
	// 3: update counter(last_tx_pk_for_nft, nft_app_log_idx)
	t int
	d interface{}
}

type nftAssetStore struct {
	tx       *tx.Transaction
	nft      *nft.Nft
	regInfo  *nft.NftRegInfo
	atHeight uint
}

type nftTxStore struct {
	tx            *tx.Transaction
	applogIdx     int
	assetID       string
	fromAddr      string
	fromBalance   *big.Float
	toAddr        string
	toBalance     *big.Float
	transferValue *big.Float
	tokenID       string
	totalSupply   *big.Float
	nftJSONInfo   string
}

type nftBalanceTSStore struct {
	txPK        uint
	blockTime   uint64
	blockIndex  uint
	assetID     string
	totalSupply *big.Float
}

type nftCounterStore struct {
	txPK      uint
	applogIdx int
}

// type nft5MigrateStore struct {
// 	newAssetAdmin string
// 	oldAssetID    string
// 	newAssetID    string
// 	txPK          uint
// 	txID          string
// }

func startNftTask() {
	nftAssetDecimals = db.GetNftAssetDecimals()
	nftTxChan := make(chan *nftTxInfo, nftChanSize)
	applogChan := make(chan *tx.Transaction, nftChanSize)
	nftStoreChan := make(chan *nftStore, nftChanSize)

	lastPk, applogIdx := db.GetLastTxPkForNft()

	go fetchNftTx(nftTxChan, applogChan, lastPk, applogIdx)
	go fetchNftAppLog(4, applogChan)

	go handleNftTx(nftTxChan, nftStoreChan, applogIdx)
	go handleNftStore(nftStoreChan)
}

func fetchNftTx(nftTxChan chan<- *nftTxInfo, applogChan chan<- *tx.Transaction, lastPk uint, applogIdx int) {
	defer mail.AlertIfErr()

	// If there are some transfers in this transaction,
	// this variable will be the last index(starts from 0).
	// If this variable is -1,
	// it means CURRENT TRANSACTION HAD BEEN HANDLED(zero transfers,
	// is nft trgistration transfer, or non-transfer actions),
	// so nextTxPK should be next pk, not the current pk.
	nextTxPK := lastPk
	if applogIdx == -1 {
		nextTxPK++
	}

	for {
		txs := db.GetNftInvocationTxs(nextTxPK, 1000)

		for i := len(txs) - 1; i >= 0; i-- {
			// cannot be app call
			if len(txs[i].Script) <= 42 ||
				txs[i].TxID == "0xb00a0d7b752ba935206e1db67079c186ba38a4696d3afe28814a4834b2254cbe" {
				txs = append(txs[:i], txs[i+1:]...)
			}
		}

		if len(txs) == 0 {
			time.Sleep(2 * time.Second)
			continue
		}

		nextTxPK = txs[len(txs)-1].ID + 1

		for _, tx := range txs {
			applogChan <- tx
		}

		for _, tx := range txs {
			for {
				// Get applicationlog from map.
				appLogResult, ok := nftAppLogs.Load(tx.TxID)
				if !ok {
					time.Sleep(5 * time.Millisecond)
					continue
				}

				nftAppLogs.Delete(tx.TxID)

				nftInfo := nftTxInfo{
					tx:           tx,
					dataStack:    smartcontract.ReadScript(tx.Script),
					appLogResult: appLogResult.(*rpc.RawApplicationLogResult),
				}

				nftTxChan <- &nftInfo
				break
			}
		}
	}
}

func fetchNftAppLog(goroutines int, applogChan <-chan *tx.Transaction) {
	defer mail.AlertIfErr()

	for i := 0; i < goroutines; i++ {
		go func(ch <-chan *tx.Transaction) {
			for tx := range ch {
				appLogResult := rpc.GetApplicationLog(int(tx.BlockIndex), tx.TxID)
				nftAppLogs.Store(tx.TxID, appLogResult)
			}
		}(applogChan)
	}
}

func handleNftTx(nftTxChan <-chan *nftTxInfo, nftStoreChan chan<- *nftStore, applogIdx int) {
	defer mail.AlertIfErr()

	for nftInfo := range nftTxChan {
		tx := nftInfo.tx
		opCodeDataStack := nftInfo.dataStack
		appLogResult := nftInfo.appLogResult

		if opCodeDataStack == nil || len(*opCodeDataStack) == 0 {
			nftStoreChan <- &nftStore{
				t: 3,
				d: nftCounterStore{
					txPK:      tx.ID,
					applogIdx: -1,
				},
			}
			continue
		}

		// It may be a nft registration transaction.
		if applogIdx == -1 && isNftRegistrationTx(tx.Script) {
			handleNftRegTx(nftStoreChan, tx, opCodeDataStack.Copy())
			// if isNftMigrateTx((tx.Script)) {
			// 	handleNftMigrate(opCodeDataStack, nftStoreChan, tx)
			// }
		} else {
			handleNftNonTxCall(nftStoreChan, tx, opCodeDataStack)

			if len(appLogResult.Executions) > 0 {
				notifs := []rpc.RawNotifications{}

				for _, exec := range appLogResult.Executions {
					if strings.Contains(exec.VMState, "FAULT") ||
						len(exec.Notifications) == 0 {
						continue
					}

					notifs = append(notifs, exec.Notifications...)
				}

				handleNftTxCall(nftStoreChan, tx, notifs, applogIdx)
			}

			// Set applogIdx to -1 to signify these transaction has been handled.
			applogIdx = -1
			nftStoreChan <- &nftStore{
				t: 3,
				d: nftCounterStore{
					txPK:      tx.ID,
					applogIdx: applogIdx,
				},
			}
		}
	}
}

// func handleMigrate(opCodeDataStack *smartcontract.DataStack, nftStoreChan chan<- *nftStore, tx *tx.Transaction) {
// 	scriptHash := opCodeDataStack.PopData()
// 	oldAssetID := util.GetAssetIDFromScriptHash(scriptHash)
// 	if len(oldAssetID) != 40 {
// 		nftStoreChan <- &nftStore{
// 			t: 3,
// 			d: nftCounterStore{
// 				txPK:      tx.ID,
// 				applogIdx: -1,
// 			},
// 		}
// 		return
// 	}

// 	newAssetAdmin, newAssetID, ok := handleNftRegTx(nftStoreChan, tx, opCodeDataStack)
// 	if !ok {
// 		nftStoreChan <- &nftStore{
// 			t: 3,
// 			d: nftCounterStore{
// 				txPK:      tx.ID,
// 				applogIdx: -1,
// 			},
// 		}
// 		return
// 	}

// 	nftStoreChan <- &nftStore{
// 		t: 4,
// 		d: nftMigrateStore{
// 			newAssetAdmin: newAssetAdmin,
// 			oldAssetID:    oldAssetID,
// 			newAssetID:    newAssetID,
// 			txPK:          tx.ID,
// 			txID:          tx.TxID,
// 		},
// 	}
// }

func handleNftStore(nftStore <-chan *nftStore) {
	defer mail.AlertIfErr()

	for s := range nftStore {
		txPK := uint(0)

		switch s.t {
		case 0:
			txPK = handleNftAssetStore(s)
		case 1:
			txPK = handleNftTxStore(s)
		case 2:
			txPK = handleNftBalanceTotalSupplyStore(s)
		case 3:
			txPK = handleNftCounterStore(s)
		case 4:
			// txPK = handleNftMigrate(s)
		default:
			err := fmt.Errorf("error nft store type %d: %+v", s.t, s.d)
			panic(err)
		}

		showNftProgress(txPK)
	}
}

func handleNftAssetStore(s *nftStore) uint {
	d, ok := s.d.(nftAssetStore)
	if !ok {
		err := fmt.Errorf("error nft store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.InsertNftAsset(d.tx,
		d.nft,
		d.regInfo,
		d.atHeight)
	if err != nil {
		panic(err)
	}

	return d.tx.ID
}

func handleNftTxStore(s *nftStore) uint {
	d, ok := s.d.(nftTxStore)
	if !ok {
		err := fmt.Errorf("error nft store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.InsertNftTransaction(
		d.tx,
		d.applogIdx,
		d.assetID,
		d.fromAddr,
		d.fromBalance,
		d.toAddr,
		d.toBalance,
		d.transferValue,
		d.tokenID,
		d.totalSupply,
		d.nftJSONInfo,
	)
	if err != nil {
		panic(err)
	}

	return d.tx.ID
}

func handleNftBalanceTotalSupplyStore(s *nftStore) uint {
	d, ok := s.d.(nftBalanceTSStore)
	if !ok {
		err := fmt.Errorf("error nft store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.UpdateNftTotalSupplyAndAddrAsset(
		d.blockTime,
		d.blockIndex,
		d.assetID,
		d.totalSupply)
	if err != nil {
		panic(err)
	}

	return d.txPK
}

func handleNftCounterStore(s *nftStore) uint {
	d, ok := s.d.(nftCounterStore)
	if !ok {
		err := fmt.Errorf("error nft store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.UpdateLastTxPkForNft(d.txPK, d.applogIdx)
	if err != nil {
		panic(err)
	}

	return d.txPK
}

func handleNftRegTx(nftStoreChan chan<- *nftStore, tx *tx.Transaction, opCodeDataStack *smartcontract.DataStack) (string, string, bool) {
	adminAddr, ok := getCallerAddr(tx)
	if !ok {
		return "", "", false
	}

	regInfo, ok := nft.GetNftRegInfo(tx.TxID, opCodeDataStack)
	if !ok {
		return "", "", false
	}

	assetID := util.GetAssetIDFromScriptHash(regInfo.ScriptHash)
	if _, ok := nftAssetDecimals[assetID]; ok {
		return util.GetAddressFromScriptHash(adminAddr), assetID, true
	}

	// Get nft definitions to make sure it is nft.
	nft, atHeight, ok := queryNftAssetInfo(tx, regInfo.ScriptHash, adminAddr)
	if !ok {
		return "", "", false
	}

	// Cache total supply.
	cache.UpdateAssetTotalSupply(nft.AssetID, nft.TotalSupply, atHeight)

	nftStoreChan <- &nftStore{
		t: 0,
		d: nftAssetStore{
			tx:       tx,
			nft:      nft,
			regInfo:  regInfo,
			atHeight: atHeight,
		},
	}

	nftAssetDecimals[nft.AssetID] = nft.Decimals
	return util.GetAddressFromScriptHash(adminAddr), assetID, true
}

// func handleNftMigrate(s *nftStore) uint {
// 	d, ok := s.d.(nftMigrateStore)
// 	if !ok {
// 		err := fmt.Errorf("err nft migrate store type %d: %+v", s.t, s.d)
// 		panic(err)
// 	}

// 	err := db.HandleNftMigrate(d.newAssetAdmin, d.oldAssetID, d.newAssetID, d.txPK, d.txID)
// 	if err != nil {
// 		panic(err)
// 	}

// 	return d.txPK
// }

func handleNftNonTxCall(nftStoreChan chan<- *nftStore, tx *tx.Transaction, opCodeDataStack *smartcontract.DataStack) {
	// At least two commands are required(opCode and its related data).
	for len(*opCodeDataStack) >= 2 {
		opCode, data := opCodeDataStack.PopItem()

		if opCode != 0x67 { // APPCALL
			continue
		}

		scriptHash := data
		if len(scriptHash) != 20 {
			continue
		}

		method := opCodeDataStack.PopData()
		// Will use 'getapplicationlog' for 'transfer' record so omit this type.
		if len(method) == 0 || reflect.DeepEqual(method, []byte("transfer")) {
			continue
		}

		totalSupply, ok := queryNftTotalSupply(tx.BlockIndex, tx.BlockTime, scriptHash)
		if !ok {
			continue
		}

		assetID := util.GetAssetIDFromScriptHash(scriptHash)

		nftStoreChan <- &nftStore{
			t: 2,
			d: nftBalanceTSStore{
				txPK:        tx.ID,
				blockTime:   tx.BlockTime,
				blockIndex:  tx.BlockIndex,
				assetID:     assetID,
				totalSupply: totalSupply,
			},
		}
	}
}

func handleNftTxCall(nftStoreChan chan<- *nftStore, tx *tx.Transaction, notifs []rpc.RawNotifications, applogIdx int) {
	// Get all transfers.
	for applogIdx++; applogIdx < len(notifs); applogIdx++ {
		notification := notifs[applogIdx]
		state := notification.State

		if state == nil || state.Type != "Array" {
			continue
		}

		stackValues := state.GetArray()

		if len(stackValues) != 5 {
			continue
		}

		if stackValues[0].Type != "ByteArray" || stackValues[0].Value != "7472616e73666572" {
			continue
		}

		if stackValues[1].Type == "Boolean" ||
			stackValues[2].Type == "Boolean" {
			continue
		}

		if len(stackValues[1].Value.(string)) == 0 &&
			len(stackValues[2].Value.(string)) == 0 {
			continue
		}

		fromSc := stackValues[1].Value.(string)
		toSc := stackValues[2].Value.(string)
		valType := stackValues[3].Type
		val := stackValues[3].Value.(string)
		tokenIDType := stackValues[4].Type
		tokenIDStr := stackValues[4].Value.(string)
		assetID := notification.Contract[2:]

		// Check if this is a valid assetID.
		if _, ok := nftAssetDecimals[assetID]; !ok {
			continue
		}

		recordNftTransfer(nftStoreChan, tx, assetID, fromSc, toSc, val, valType, tokenIDStr, tokenIDType, applogIdx)
	}
}

func recordNftTransfer(nftStoreChan chan<- *nftStore, tx *tx.Transaction, assetID, fromSc, toSc, val, valType, tokenIDStr, tokenIDType string, applogIdx int) {
	scriptHash := util.GetScriptHashFromAssetID(assetID)

	// 'From' address may be empty(when issuing an asset).
	from, _ := hex.DecodeString(fromSc)
	fromAddr := util.GetAddressFromScriptHash(from)
	to, _ := hex.DecodeString(toSc)
	toAddr := util.GetAddressFromScriptHash(to)

	if len(fromAddr) > 128 || len(toAddr) > 128 {
		log.Error.Printf("TxID: %s, from=%s, to=%s", tx.TxID, fromAddr, toAddr)
		return
	}

	transferValue, ok := getNftTransferValue(assetID, val, valType)
	if !ok {
		return
	}

	tokenID, ok := getNftTokenID(tokenIDStr, tokenIDType)
	if !ok {
		return
	}

	// Get nft asset balance of this two addresses.
	balances, ok := queryNftBalances(tx.BlockIndex, scriptHash, assetID, [][]byte{from, to})
	if !ok {
		return
	}

	fromBalance := balances[0]
	toBalance := balances[1]

	// Handle possibility of storage injection attack.
	var totalSupply *big.Float
	if toSc == "746f74616c537570706c79" {
		totalSupply, _ = queryNftTotalSupply(tx.BlockIndex, tx.BlockTime, scriptHash)
	}

	nftJSONInfo := ""
	// Query NFT token info if it's new.
	if tokenID != "" && !db.NftTokenExists(assetID, tokenID) {
		nftJSONInfo, _ = queryNFTTokenInfo(tx, scriptHash, tokenID)
	}

	nftStoreChan <- &nftStore{
		t: 1,
		d: nftTxStore{
			tx:            tx,
			applogIdx:     applogIdx,
			assetID:       assetID,
			fromAddr:      fromAddr,
			fromBalance:   fromBalance,
			toAddr:        toAddr,
			toBalance:     toBalance,
			transferValue: transferValue,
			tokenID:       tokenID,
			totalSupply:   totalSupply,
			nftJSONInfo:   nftJSONInfo,
		},
	}
}

func queryNFTTokenInfo(tx *tx.Transaction, scriptHash []byte, tokenID string) (string, bool) {
	scripts := smartcontract.CreateNftPropertiesScript(scriptHash, tokenID)

	minHeight := getMinHeight(tx.BlockIndex)

	// When there are more than one backend fullnodes,
	// sometimes if node A received a new block which contains
	// NFT transfers, but at the same time B just synced to the same height
	// and haven't handle the same NFT transfers, in this case(little possibility),
	// the 'properties' query would fail, so try for three times to make sure if
	// in this case, or the tokenID is just invalid.

	for i := 0; i < 3; i++ {
		result := rpc.SmartContractRPCCall(minHeight, scripts)
		if result == nil || strings.Contains(result.State, "FAULT") {
			time.Sleep(1 * time.Second)
			continue
		}
		if len(result.Stack) < 1 {
			time.Sleep(1 * time.Second)
			continue
		}

		propertiesBytesStr, ok := result.Stack[0].Value.(string)
		if !ok {
			time.Sleep(1 * time.Second)
			continue
		}

		propertiesBytes, _ := hex.DecodeString(propertiesBytesStr)
		propertiesJSON := string(propertiesBytes)

		if !utf8.ValidString(propertiesJSON) {
			propertiesJSON = ""
		}

		return propertiesJSON, true
	}

	return "", false
}

func getNftTransferValue(assetID string, val string, valType string) (*big.Float, bool) {
	value, ok := extractValue(val, valType)
	if !ok {
		return nil, false
	}

	return getNftReadableValue(assetID, value), true
}

func getNftTokenID(str, valType string) (string, bool) {
	switch valType {
	case "Integer":
		return str, true
	case "ByteArray":
		bytes, err := hex.DecodeString(str)
		if err != nil {
			log.Error.Printf("getNftTokenID(%s, %s) failed: %v", str, valType, err)
			return "", false
		}

		bint := new(big.Int).SetBytes(util.ReverseBytes(bytes))
		return bint.String(), true
	default:
		return "", false
	}
}

func isNftRegistrationTx(script string) bool {
	// name
	if strings.Contains(script, "6e616d65") &&
		// symbol
		strings.Contains(script, "73796d626f6c") &&
		// totalSupply
		strings.Contains(script, "746f74616c537570706c79") &&
		// decimals
		strings.Contains(script, "646563696d616c73") &&
		// transfer
		strings.Contains(script, "7472616e73666572") &&
		// ownerOf
		strings.Contains(script, "6f776e65724f66") &&
		// balanceOf
		strings.Contains(script, "62616c616e63654f66") &&
		// tokensOf
		strings.Contains(script, "746f6b656e734f66") &&
		// properties
		strings.Contains(script, "70726f70657274696573") {
		return true
	}

	return false
}

// func isNftMigrateTx(script string) bool {
// 	// Neo.Contract.Migrate: 4e656f2e436f6e74726163742e4d696772617465
// 	keyword := "68144e656f2e436f6e74726163742e4d696772617465"
// 	return strings.Contains(script, keyword)
// }

func queryNftAssetInfo(tx *tx.Transaction, scriptHash []byte, addrBytes []byte) (*nft.Nft, uint, bool) {
	assetID := util.GetAssetIDFromScriptHash(scriptHash)
	adminAddr := util.GetAddressFromScriptHash(addrBytes)

	scripts := createSCSB(scriptHash, "name", nil)
	scripts += createSCSB(scriptHash, "symbol", nil)
	scripts += createSCSB(scriptHash, "decimals", nil)
	scripts += createSCSB(scriptHash, "totalSupply", nil)

	minHeight := getMinHeight(tx.BlockIndex)
	result := rpc.SmartContractRPCCall(minHeight, scripts)
	if result == nil || strings.Contains(result.State, "FAULT") {
		return nil, 0, false
	}
	if len(result.Stack) < 4 {
		return nil, 0, false
	}

	nameBytesStr, ok := result.Stack[0].Value.(string)
	if !ok {
		return nil, 0, false
	}
	nameBytes, err := hex.DecodeString(nameBytesStr)
	if err != nil {
		return nil, 0, false
	}
	name := strings.Replace(string(nameBytes), "'", "\\'", -1)
	if name == "" {
		return nil, 0, false
	}

	symbolBytesStr, ok := result.Stack[1].Value.(string)
	if !ok {
		return nil, 0, false
	}
	symbolBytes, _ := hex.DecodeString(symbolBytesStr)
	symbol := string(symbolBytes)
	if symbol == "" {
		return nil, 0, false
	}

	decimalsHexStr, ok := result.Stack[2].Value.(string)
	if !ok {
		return nil, 0, false
	}
	decimals := util.HexToBigInt(decimalsHexStr).Int64()
	if decimals < 0 || decimals > 8 {
		return nil, 0, false
	}

	totalSupply, ok := extractValue(result.Stack[3].Value, result.Stack[3].Type)
	if !ok {
		return nil, 0, false
	}
	totalSupply = new(big.Float).SetPrec(256).Quo(totalSupply, big.NewFloat(math.Pow10(int(decimals))))

	nft := &nft.Nft{
		AssetID:          assetID,
		AdminAddress:     adminAddr,
		Name:             name,
		Symbol:           symbol,
		Decimals:         uint8(decimals),
		TotalSupply:      totalSupply,
		TxID:             tx.TxID,
		BlockIndex:       tx.BlockIndex,
		BlockTime:        tx.BlockTime,
		Addresses:        0,
		HoldingAddresses: 0,
		Transfers:        0,
	}

	return nft, uint(minHeight), true
}

func createNftBalanceSCSB(scriptHash []byte, addrBytes []byte) string {
	if len(addrBytes) == 0 {
		return ""
	}

	return createSCSB(scriptHash, "balanceOf", [][]byte{addrBytes})
}

func queryNftCallerBalance(txBlockIndex uint, blockTime uint64, scriptHash []byte, callerAddrBytes []byte) (*big.Float, bool) {
	assetID := util.GetAssetIDFromScriptHash(scriptHash)
	callerAddr := util.GetAddressFromScriptHash(callerAddrBytes)

	// Query from cache.
	if cached, ok := cache.GetAddrAsset(callerAddr, assetID); ok {
		// If cache valid.
		if cached.BlockIndex > txBlockIndex {
			return cached.Balance, true
		}
	}

	decimals, ok := nftAssetDecimals[assetID]
	if !ok {
		return big.NewFloat(0), false
	}

	scripts := createSCSB(scriptHash, "balanceOf", [][]byte{callerAddrBytes})

	minHeight := rpc.BestHeight.Get()
	result := rpc.SmartContractRPCCall(minHeight, scripts)
	if result == nil ||
		strings.Contains(result.State, "FAULT") ||
		result.Stack == nil ||
		len(result.Stack) == 0 {
		return big.NewFloat(0), false
	}

	callerBalance, ok := extractValue(result.Stack[0].Value, result.Stack[0].Type)
	if !ok {
		return big.NewFloat(0), false
	}
	callerBalance = new(big.Float).SetPrec(256).Quo(callerBalance, big.NewFloat(math.Pow10(int(decimals))))

	return callerBalance, true
}

func queryNftBalances(txBlockIndex uint, scriptHash []byte, assetID string, addrBytesList [][]byte) ([]*big.Float, bool) {
	// Check if this is a valid assetID.
	if _, ok := nftAssetDecimals[assetID]; !ok {
		return nil, false
	}

	balances := make([]*big.Float, len(addrBytesList))

	scsb := ""

	for idx, addrBytes := range addrBytesList {
		if len(addrBytes) == 0 {
			balances[idx] = big.NewFloat(0)
		} else {
			// Check cached value.
			addr := util.GetAddressFromScriptHash(addrBytes)
			if cached, ok := cache.GetAddrAsset(addr, assetID); ok {
				// If cache valid.
				if cached.BlockIndex > txBlockIndex {
					balances[idx] = cached.Balance
					continue
				}
			}

			// Query balance
			scsb += createNftBalanceSCSB(scriptHash, addrBytes)
		}
	}

	minHeight := rpc.BestHeight.Get()
	result := rpc.SmartContractRPCCall(minHeight, scsb)

	// If this nft asset is broken(for example forgot to check 'need storage').
	if result == nil || strings.Contains(result.State, "FAULT") {
		return nil, false
	}

	idx := 0

	for i := 0; i < len(addrBytesList); i++ {
		addrBytes := addrBytesList[i]

		if len(addrBytes) == 0 || balances[i] != nil {
			continue
		}

		balance, ok := extractValue(result.Stack[idx].Value, result.Stack[idx].Type)
		if !ok {
			continue
		}
		balances[i] = getNftReadableValue(assetID, balance)
		idx++
	}

	return balances, true
}

func queryNftTotalSupply(txBlockIndex uint, blockTime uint64, scriptHash []byte) (*big.Float, bool) {
	assetID := util.GetAssetIDFromScriptHash(scriptHash)

	decimals, ok := nftAssetDecimals[assetID]
	if !ok {
		return nil, false
	}

	// Query from cache
	totalSupply, atIndex, ok := cache.GetAssetTotalSupply(assetID)
	if ok && atIndex > txBlockIndex {
		return totalSupply, true
	}

	totalSupplyScsb := createSCSB(scriptHash, "totalSupply", nil)
	minHeight := rpc.BestHeight.Get()
	result := rpc.SmartContractRPCCall(minHeight, totalSupplyScsb)
	if result == nil || strings.Contains(result.State, "FAULT") {
		return nil, false
	}

	if len(result.Stack) == 0 {
		return nil, false
	}

	// Get the first valid result
	// Terrible tx example:
	/*
		curl -X POST \
			http://xxx.xxx.xxx.xxx:10332 \
			-d '{
			"jsonrpc": "2.0",
			"method": "invokefunction",
			"params": [
				"c54fc1e02a674ce2de52493b3138fb80ccff5a6e",
				"totalSupply"],
			"id": 1
		}'
	*/
	ok = false
	for _, stack := range result.Stack {
		totalSupply, ok = extractValue(stack.Value, stack.Type)
		if ok {
			totalSupply = new(big.Float).SetPrec(256).Quo(totalSupply, big.NewFloat(math.Pow10(int(decimals))))
			break
		}
	}

	if !ok {
		return nil, false
	}

	// Update cacheed value
	cache.UpdateAssetTotalSupply(assetID, totalSupply, uint(minHeight))

	return totalSupply, true
}

func getNftReadableValue(assetID string, balance *big.Float) *big.Float {
	zeroValue := big.NewFloat(0)
	if balance.Cmp(zeroValue) == 0 {
		return zeroValue
	}

	decimals, ok := nftAssetDecimals[assetID]
	if !ok {
		panic("Failed to get decimals of nft asset: " + assetID)
	}

	return new(big.Float).SetPrec(256).Quo(balance, big.NewFloat(math.Pow10(int(decimals))))
}

func showNftProgress(txPk uint) {
	if maxNftPK == 0 || nftMaxPkShouldRefresh {
		nftMaxPkShouldRefresh = false
		maxNftPK = db.GetMaxNonEmptyScriptTxPk()
	}

	now := time.Now()
	if nftProgress.LastOutputTime == (time.Time{}) {
		nftProgress.LastOutputTime = now
	}
	if txPk < maxNftPK && now.Sub(nftProgress.LastOutputTime) < time.Second {
		return
	}

	GetEstimatedRemainingTime(int64(txPk), int64(maxNftPK), &nftProgress)
	if nftProgress.Percentage.Cmp(big.NewFloat(100)) == 0 &&
		bProgress.Finished {
		nftProgress.Finished = true
	}

	log.Printf("%sProgress of nft: %d/%d, %.4f%%\n",
		nftProgress.RemainingTimeStr,
		txPk,
		maxNftPK,
		nftProgress.Percentage)
	nftProgress.LastOutputTime = now

	// Send mail if fully synced
	if nftProgress.Finished && !nftProgress.MailSent {
		nftProgress.MailSent = true

		// If sync lasts shortly, do not send mail
		if time.Since(nftProgress.InitTime) < time.Minute*5 {
			return
		}

		msg := fmt.Sprintf("Init time: %v\nEnd Time: %v\n", nftProgress.InitTime, time.Now())
		mail.SendNotify("nft TX Fully Synced", msg)
	}
}
