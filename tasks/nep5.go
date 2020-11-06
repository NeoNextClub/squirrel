/*
To restart this task from beginning, execute the following sqls:

DELETE FROM `addr_asset` WHERE LENGTH(`asset_id`) = 40;
DELETE FROM `addr_tx` WHERE `asset_type` = 'nep5';
UPDATE `address` SET `trans_nep5` = 0 WHERE 1=1;
UPDATE `counter` SET
	`last_tx_pk_for_nep5` = 0,
	`app_log_idx` = -1
WHERE `id` = 1;
TRUNCATE TABLE `nep5`;
TRUNCATE TABLE `nep5_reg_info`;
TRUNCATE TABLE `nep5_tx`;
TRUNCATE TABLE `nep5_migrate`;
UPDATE `counter` SET `nep5_tx_pk_for_addr_tx`=0 WHERE `id`=1;

To check if rpc node has enabled smart contract log,
check if the first nep5 transfer exists:
mainnet:
	txID: 0xc920b2192e74eda4ca6140510813aa40fef1767d00c152aa6f8027c24bdf14f2
	blockIndex: 1444843
testnet:
	txID: 0xd355c4cf3a58859cd72c28ce362d727ed7dc2d68ae049ca9356fc0a6c21a8f45
	blockIndex: 446369

run the following curl command for mainnet(replace IP address):

curl -X POST \
  http://xxx.xxx.xxx.xxx:10332 \
  -H 'Content-Type: application/json' \
  -H 'cache-control: no-cache' \
  -d '{
    "jsonrpc": "2.0",
    "method": "getapplicationlog",
    "params": [
        "0xc920b2192e74eda4ca6140510813aa40fef1767d00c152aa6f8027c24bdf14f2"
    ],
    "id": 1
}'

run the following curl command for testnet(replace IP address):

curl -X POST \
  http://xxx.xxx.xxx.xxx:10332 \
  -H 'Content-Type: application/json' \
  -H 'cache-control: no-cache' \
  -d '{
    "jsonrpc": "2.0",
    "method": "getapplicationlog",
    "params": [
        "0xd355c4cf3a58859cd72c28ce362d727ed7dc2d68ae049ca9356fc0a6c21a8f45"
    ],
    "id": 1
}'

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
	"squirrel/smartcontract"
	"strings"
	"sync"
	"time"

	"squirrel/addr"
	"squirrel/db"
	"squirrel/nep5"
	"squirrel/rpc"
	"squirrel/tx"
	"squirrel/util"
)

const nep5ChanSize = 5000

var maxVal *big.Float

var (
	// Nep5MaxPkShouldRefresh indicates if highest pk should be refreshed
	Nep5MaxPkShouldRefresh bool

	// Cache decimals of nep5 asset
	nep5AssetDecimals map[string]uint8
	nProgress         = Progress{}
	maxNep5PK         uint

	// appLogs stores txid with its applicationlog rpc response
	appLogs sync.Map
)

type nep5TxInfo struct {
	tx           *tx.Transaction
	dataStack    *smartcontract.DataStack
	appLogResult *rpc.RawApplicationLogResult
}

type nep5Store struct {
	// 0: nep5 reg
	// 1: nep5 tx
	// 2: nep5 addr balance and total supply
	// 3: update counter(last_tx_pk_for_nep5, app_log_idx)
	t int
	d interface{}
}

type nep5AssetStore struct {
	tx        *tx.Transaction
	nep5      *nep5.Nep5
	regInfo   *nep5.RegInfo
	addrAsset *addr.Asset
	atHeight  uint
}

type nep5TxStore struct {
	tx            *tx.Transaction
	applogIdx     int
	assetID       string
	fromAddr      string
	fromBalance   *big.Float
	toAddr        string
	toBalance     *big.Float
	transferValue *big.Float
	totalSupply   *big.Float
}

type nep5BalanceTSStore struct {
	txPK        uint
	blockTime   uint64
	blockIndex  uint
	addr        string
	balance     *big.Float
	assetID     string
	totalSupply *big.Float
}

type nep5CounterStore struct {
	txPK      uint
	applogIdx int
}

type nep5MigrateStore struct {
	newAssetAdmin string
	oldAssetID    string
	newAssetID    string
	txPK          uint
	txID          string
}

func init() {
	maxVal, _ = new(big.Float).SetPrec(256).SetString("99999999999999999999999999999999999.99999999")
}

func startNep5Task() {
	nep5AssetDecimals = db.GetNep5AssetDecimals()
	nep5TxChan := make(chan *nep5TxInfo, nep5ChanSize)
	applogChan := make(chan *tx.Transaction, nep5ChanSize)
	nep5StoreChan := make(chan *nep5Store, nep5ChanSize)

	lastPk, applogIdx := db.GetLastTxPkForNep5()

	go fetchNep5Tx(nep5TxChan, applogChan, lastPk, applogIdx)
	go fetchAppLog(4, applogChan)

	go handleNep5Tx(nep5TxChan, nep5StoreChan, applogIdx)
	go handleNep5Store(nep5StoreChan)
}

func fetchNep5Tx(nep5TxChan chan<- *nep5TxInfo, applogChan chan<- *tx.Transaction, lastPk uint, applogIdx int) {
	defer mail.AlertIfErr()

	// If there are some transfers in this transaction,
	// this variable will be the last index(starts from 0).
	// If this variable is -1,
	// it means CURRENT TRANSACTION HAD BEEN HANDLED(zero transfers,
	// is nep5 trgistration transfer, or non-transfer actions),
	// so nextTxPK should be next pk, not the current pk.
	nextTxPK := lastPk
	if applogIdx == -1 {
		nextTxPK++
	}

	for {
		txs := db.GetInvocationTxs(nextTxPK, 1000)

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
				appLogResult, ok := appLogs.Load(tx.TxID)
				if !ok {
					time.Sleep(10 * time.Millisecond)
					continue
				}

				appLogs.Delete(tx.TxID)

				nep5Info := nep5TxInfo{
					tx:           tx,
					dataStack:    smartcontract.ReadScript(tx.Script),
					appLogResult: appLogResult.(*rpc.RawApplicationLogResult),
				}

				nep5TxChan <- &nep5Info
				break
			}
		}
	}
}

func fetchAppLog(goroutines int, applogChan <-chan *tx.Transaction) {
	defer mail.AlertIfErr()

	for i := 0; i < goroutines; i++ {
		go func(ch <-chan *tx.Transaction) {
			for tx := range ch {
				appLogResult := rpc.GetApplicationLog(int(tx.BlockIndex), tx.TxID)
				appLogs.Store(tx.TxID, appLogResult)
			}
		}(applogChan)
	}
}

func handleNep5Tx(nep5TxChan <-chan *nep5TxInfo, nep5StoreChan chan<- *nep5Store, applogIdx int) {
	defer mail.AlertIfErr()

	for nep5Info := range nep5TxChan {
		tx := nep5Info.tx
		opCodeDataStack := nep5Info.dataStack
		appLogResult := nep5Info.appLogResult

		if opCodeDataStack == nil || len(*opCodeDataStack) == 0 {
			nep5StoreChan <- &nep5Store{
				t: 3,
				d: nep5CounterStore{
					txPK:      tx.ID,
					applogIdx: -1,
				},
			}
			continue
		}

		// It may be a nep5 registration transaction.
		if applogIdx == -1 && isNep5RegistrationTx(tx.Script) {
			handleNep5RegTx(nep5StoreChan, tx, opCodeDataStack.Copy())
			if isNep5MigrateTx((tx.Script)) {
				handleNep5Migrate(opCodeDataStack, nep5StoreChan, tx)
			}
		} else if applogIdx == -1 && isNep5MigrateTx(tx.Script) {
			handleNep5Migrate(opCodeDataStack, nep5StoreChan, tx)
		} else {
			handleNep5NonTxCall(nep5StoreChan, tx, opCodeDataStack)

			if len(appLogResult.Executions) > 0 {
				notifs := []rpc.RawNotifications{}

				for _, exec := range appLogResult.Executions {
					if strings.Contains(exec.VMState, "FAULT") ||
						len(exec.Notifications) == 0 {
						continue
					}

					notifs = append(notifs, exec.Notifications...)
				}

				handleNep5TxCall(nep5StoreChan, tx, notifs, applogIdx)
			}

			// Set applogIdx to -1 to signify these transaction has been handled.
			applogIdx = -1
			nep5StoreChan <- &nep5Store{
				t: 3,
				d: nep5CounterStore{
					txPK:      tx.ID,
					applogIdx: applogIdx,
				},
			}
		}
	}
}

func handleNep5Migrate(opCodeDataStack *smartcontract.DataStack, nep5StoreChan chan<- *nep5Store, tx *tx.Transaction) {
	scriptHash := opCodeDataStack.PopData()
	oldAssetID := util.GetAssetIDFromScriptHash(scriptHash)
	if len(oldAssetID) != 40 {
		nep5StoreChan <- &nep5Store{
			t: 3,
			d: nep5CounterStore{
				txPK:      tx.ID,
				applogIdx: -1,
			},
		}
		return
	}

	newAssetAdmin, newAssetID, ok := handleNep5RegTx(nep5StoreChan, tx, opCodeDataStack)
	if !ok {
		nep5StoreChan <- &nep5Store{
			t: 3,
			d: nep5CounterStore{
				txPK:      tx.ID,
				applogIdx: -1,
			},
		}
		return
	}

	nep5StoreChan <- &nep5Store{
		t: 4,
		d: nep5MigrateStore{
			newAssetAdmin: newAssetAdmin,
			oldAssetID:    oldAssetID,
			newAssetID:    newAssetID,
			txPK:          tx.ID,
			txID:          tx.TxID,
		},
	}
}

func handleNep5Store(nep5Store <-chan *nep5Store) {
	defer mail.AlertIfErr()

	for s := range nep5Store {
		txPK := uint(0)

		switch s.t {
		case 0:
			txPK = handleNep5AssetStore(s)
		case 1:
			txPK = handleNep5TxStore(s)
		case 2:
			txPK = handleNep5BalanceTotalSupplyStore(s)
		case 3:
			txPK = handleNep5CounterStore(s)
		case 4:
			txPK = handleNEP5Migrate(s)
		default:
			err := fmt.Errorf("error nep5 store type %d: %+v", s.t, s.d)
			panic(err)
		}

		showNep5Progress(txPK)
	}
}

func handleNep5AssetStore(s *nep5Store) uint {
	d, ok := s.d.(nep5AssetStore)
	if !ok {
		err := fmt.Errorf("error nep5 store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.InsertNep5Asset(d.tx,
		d.nep5,
		d.regInfo,
		d.addrAsset,
		d.atHeight)
	if err != nil {
		panic(err)
	}

	return d.tx.ID
}

func handleNep5TxStore(s *nep5Store) uint {
	d, ok := s.d.(nep5TxStore)
	if !ok {
		err := fmt.Errorf("error nep5 store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.InsertNep5transaction(d.tx,
		d.applogIdx,
		d.assetID,
		d.fromAddr,
		d.fromBalance,
		d.toAddr,
		d.toBalance,
		d.transferValue,
		d.totalSupply)
	if err != nil {
		panic(err)
	}

	return d.tx.ID
}

func handleNep5BalanceTotalSupplyStore(s *nep5Store) uint {
	d, ok := s.d.(nep5BalanceTSStore)
	if !ok {
		err := fmt.Errorf("error nep5 store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.UpdateNep5TotalSupplyAndAddrAsset(
		d.blockTime,
		d.blockIndex,
		d.addr,
		d.balance,
		d.assetID,
		d.totalSupply)
	if err != nil {
		panic(err)
	}

	return d.txPK
}

func handleNep5CounterStore(s *nep5Store) uint {
	d, ok := s.d.(nep5CounterStore)
	if !ok {
		err := fmt.Errorf("error nep5 store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.UpdateLastTxPkForNep5(d.txPK, d.applogIdx)
	if err != nil {
		panic(err)
	}

	return d.txPK
}

func handleNep5RegTx(nep5StoreChan chan<- *nep5Store, tx *tx.Transaction, opCodeDataStack *smartcontract.DataStack) (string, string, bool) {
	adminAddr, ok := getCallerAddr(tx)
	if !ok {
		return "", "", false
	}

	regInfo, ok := nep5.GetNep5RegInfo(tx.TxID, opCodeDataStack)
	if !ok {
		return "", "", false
	}

	assetID := util.GetAssetIDFromScriptHash(regInfo.ScriptHash)
	if _, ok := nep5AssetDecimals[assetID]; ok {
		return util.GetAddressFromScriptHash(adminAddr), assetID, true
	}

	// Get nep5 definitions to make sure it is nep5.
	nep5, addrAsset, atHeight, ok := queryNep5AssetInfo(tx, regInfo.ScriptHash, adminAddr)
	if !ok {
		return "", "", false
	}

	// Cache total supply.
	cache.UpdateAssetTotalSupply(nep5.AssetID, nep5.TotalSupply, atHeight)

	nep5StoreChan <- &nep5Store{
		t: 0,
		d: nep5AssetStore{
			tx:        tx,
			nep5:      nep5,
			regInfo:   regInfo,
			addrAsset: addrAsset,
			atHeight:  atHeight,
		},
	}

	nep5AssetDecimals[nep5.AssetID] = nep5.Decimals
	return util.GetAddressFromScriptHash(adminAddr), assetID, true
}

func handleNEP5Migrate(s *nep5Store) uint {
	d, ok := s.d.(nep5MigrateStore)
	if !ok {
		err := fmt.Errorf("err nep5 migrate store type %d: %+v", s.t, s.d)
		panic(err)
	}

	err := db.HandleNEP5Migrate(d.newAssetAdmin, d.oldAssetID, d.newAssetID, d.txPK, d.txID)
	if err != nil {
		panic(err)
	}

	return d.txPK
}

func handleNep5NonTxCall(nep5StoreChan chan<- *nep5Store, tx *tx.Transaction, opCodeDataStack *smartcontract.DataStack) {
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

		// Query totalSupply and caller's balance.
		callerAddr, ok := getCallerAddr(tx)
		if !ok {
			continue
		}

		totalSupply, ok := queryNep5TotalSupply(tx.BlockIndex, tx.BlockTime, scriptHash)
		if !ok {
			continue
		}

		callerBalance, ok := queryCallerBalance(tx.BlockIndex, tx.BlockTime, scriptHash, callerAddr)
		if !ok || callerBalance.Cmp(big.NewFloat(0)) != 1 {
			continue
		}

		callerAddrStr := util.GetAddressFromScriptHash(callerAddr)
		assetID := util.GetAssetIDFromScriptHash(scriptHash)

		nep5StoreChan <- &nep5Store{
			t: 2,
			d: nep5BalanceTSStore{
				txPK:        tx.ID,
				blockTime:   tx.BlockTime,
				blockIndex:  tx.BlockIndex,
				addr:        callerAddrStr,
				balance:     callerBalance,
				assetID:     assetID,
				totalSupply: totalSupply,
			},
		}
	}
}

func handleNep5TxCall(nep5StoreChan chan<- *nep5Store, tx *tx.Transaction, notifs []rpc.RawNotifications, applogIdx int) {
	// Get all transfers.
	for applogIdx++; applogIdx < len(notifs); applogIdx++ {
		notification := notifs[applogIdx]
		state := notification.State

		if state == nil || state.Type != "Array" {
			continue
		}

		stackValues := state.GetArray()

		if len(stackValues) != 4 {
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
		assetID := notification.Contract[2:]

		// Check if this is a valid assetID.
		if _, ok := nep5AssetDecimals[assetID]; !ok {
			continue
		}

		recordNep5Transfer(nep5StoreChan, tx, assetID, fromSc, toSc, val, valType, applogIdx)
		scanAttrAddrBalance(nep5StoreChan, tx, assetID)
	}
}

func scanAttrAddrBalance(nep5StoreChan chan<- *nep5Store, tx *tx.Transaction, assetID string) {
	attrs := db.GetTxAttrs(tx.TxID)
	if len(attrs) == 0 {
		return
	}

	addrScriptHash := ""

	for _, attr := range attrs {
		if attr.Usage == "Script" {
			addrScriptHash = attr.Data
			break
		}
	}

	if addrScriptHash == "" {
		return
	}

	addrSC, err := hex.DecodeString(addrScriptHash)
	if err != nil {
		return
	}

	addr := util.GetAddressFromScriptHash(addrSC)
	callerBalance, ok := queryCallerBalance(tx.BlockIndex, tx.BlockTime, util.GetScriptHashFromAssetID(assetID), addrSC)
	if !ok || callerBalance.Cmp(big.NewFloat(0)) != 1 {
		return
	}

	nep5StoreChan <- &nep5Store{
		t: 2,
		d: nep5BalanceTSStore{
			txPK:        tx.ID,
			blockTime:   tx.BlockTime,
			blockIndex:  tx.BlockIndex,
			addr:        addr,
			balance:     callerBalance,
			assetID:     assetID,
			totalSupply: nil,
		},
	}
}

func recordNep5Transfer(nep5StoreChan chan<- *nep5Store, tx *tx.Transaction, assetID string, fromSc string, toSc string, val string, valType string, applogIdx int) {
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

	transferValue, ok := getTransferValue(assetID, val, valType)
	if !ok {
		return
	}

	// Get nep5 asset balance of this two addresses.
	balances, ok := queryBalances(tx.BlockIndex, scriptHash, assetID, [][]byte{from, to})
	if !ok {
		return
	}

	fromBalance := balances[0]
	toBalance := balances[1]

	// Handle possibility of storage injection attack.
	var totalSupply *big.Float
	if toSc == "746f74616c537570706c79" || tx.BlockIndex > 6200000 {
		totalSupply, _ = queryNep5TotalSupply(tx.BlockIndex, tx.BlockTime, scriptHash)
	}

	nep5StoreChan <- &nep5Store{
		t: 1,
		d: nep5TxStore{
			tx:            tx,
			applogIdx:     applogIdx,
			assetID:       assetID,
			fromAddr:      fromAddr,
			fromBalance:   fromBalance,
			toAddr:        toAddr,
			toBalance:     toBalance,
			transferValue: transferValue,
			totalSupply:   totalSupply,
		},
	}
}

func getTransferValue(assetID string, val string, valType string) (*big.Float, bool) {
	value, ok := extractValue(val, valType)
	if !ok {
		return nil, false
	}

	return getReadableValue(assetID, value), true
}

func extractValue(val interface{}, valType string) (*big.Float, bool) {
	switch valType {
	case "Integer":
		v, ok := new(big.Float).SetPrec(256).SetString(val.(string))
		return v, ok
	case "ByteArray":
		valueBytes, err := hex.DecodeString(val.(string))
		if err != nil {
			return nil, false
		}

		return util.BytesToBigFloat(valueBytes), true
	case "Array":
		arr := val.([]interface{})
		if len(arr) == 0 {
			return big.NewFloat(0), true
		}
		if len(arr) == 2 {
			return extractValue(arr[0], arr[1].(string))
		}

		return nil, false
	default:
		return nil, false
	}
}

func getCallerAddr(tx *tx.Transaction) ([]byte, bool) {
	txScrpits, err := db.GetTxScripts(tx.TxID)
	if err != nil {
		panic(err)
	}

	if txScrpits == nil || txScrpits[0].Verification == "" {
		return nil, false
	}

	verification, _ := hex.DecodeString(txScrpits[0].Verification)
	callerAddr := util.GetScriptHash(verification)

	return callerAddr, true
}

func isNep5RegistrationTx(script string) bool {
	// totalSupply
	if strings.Contains(script, "746f74616c537570706c79") &&
		// name
		strings.Contains(script, "6e616d65") &&
		// symbol
		strings.Contains(script, "73796d626f6c") &&
		// decimals
		strings.Contains(script, "646563696d616c73") {
		// Exclude nft keywords.
		// ownerOf
		if strings.Contains(script, "6f776e65724f66") &&
			// tokensOf
			strings.Contains(script, "746f6b656e734f66") &&
			// properties
			strings.Contains(script, "70726f70657274696573") {
			return false
		}
		return true
	}

	return false
}

func isNep5MigrateTx(script string) bool {
	// Neo.Contract.Migrate: 4e656f2e436f6e74726163742e4d696772617465
	keyword := "68144e656f2e436f6e74726163742e4d696772617465"
	return strings.Contains(script, keyword)
}

func queryNep5AssetInfo(tx *tx.Transaction, scriptHash []byte, addrBytes []byte) (*nep5.Nep5, *addr.Asset, uint, bool) {
	assetID := util.GetAssetIDFromScriptHash(scriptHash)
	adminAddr := util.GetAddressFromScriptHash(addrBytes)

	scripts := createSCSB(scriptHash, "name", nil)
	scripts += createSCSB(scriptHash, "symbol", nil)
	scripts += createSCSB(scriptHash, "decimals", nil)
	scripts += createSCSB(scriptHash, "totalSupply", nil)
	scripts += createSCSB(scriptHash, "balanceOf", [][]byte{addrBytes})

	minHeight := getMinHeight(tx.BlockIndex)
	result := rpc.SmartContractRPCCall(minHeight, scripts)
	if result == nil || strings.Contains(result.State, "FAULT") {
		return nil, nil, 0, false
	}
	if len(result.Stack) < 5 {
		return nil, nil, 0, false
	}

	nameBytesStr, ok := result.Stack[0].Value.(string)
	if !ok {
		return nil, nil, 0, false
	}
	nameBytes, err := hex.DecodeString(nameBytesStr)
	if err != nil {
		return nil, nil, 0, false
	}
	name := strings.Replace(string(nameBytes), "'", "\\'", -1)
	if name == "" {
		return nil, nil, 0, false
	}

	symbolBytesStr, ok := result.Stack[1].Value.(string)
	if !ok {
		return nil, nil, 0, false
	}
	symbolBytes, _ := hex.DecodeString(symbolBytesStr)
	symbol := string(symbolBytes)
	if symbol == "" {
		return nil, nil, 0, false
	}
	if len(symbol) > 16 {
		symbol = symbol[:16]
	}

	decimalsHexStr, ok := result.Stack[2].Value.(string)
	if !ok {
		return nil, nil, 0, false
	}
	decimals := util.HexToBigInt(decimalsHexStr).Int64()
	if decimals < 0 {
		return nil, nil, 0, false
	}

	totalSupply, ok := extractValue(result.Stack[3].Value, result.Stack[3].Type)
	if !ok {
		return nil, nil, 0, false
	}
	totalSupply = new(big.Float).SetPrec(256).Quo(totalSupply, big.NewFloat(math.Pow10(int(decimals))))
	if totalSupply.Cmp(maxVal) > 0 {
		totalSupply = big.NewFloat(0)
	}

	adminBalance, ok := extractValue(result.Stack[4].Value, result.Stack[4].Type)
	if !ok {
		return nil, nil, 0, false
	}
	if adminBalance.Cmp(big.NewFloat(0)) == 1 {
		adminBalance = new(big.Float).SetPrec(256).Quo(adminBalance, big.NewFloat(math.Pow10(int(decimals))))
	}
	if adminBalance.Cmp(maxVal) > 0 {
		adminBalance = big.NewFloat(0)
	}

	addrHasBalance := adminBalance.Cmp(big.NewFloat(0))

	nep5 := &nep5.Nep5{
		AssetID:          assetID,
		AdminAddress:     adminAddr,
		Name:             name,
		Symbol:           symbol,
		Decimals:         uint8(decimals),
		TotalSupply:      totalSupply,
		TxID:             tx.TxID,
		BlockIndex:       tx.BlockIndex,
		BlockTime:        tx.BlockTime,
		Addresses:        uint64(addrHasBalance),
		HoldingAddresses: uint64(addrHasBalance),
		Transfers:        0,
	}

	var addrAsset *addr.Asset

	// Maybe admin does not have any balance.
	// Admin may have responsibility only for calling functions,
	// itself is not the one who to 'deploy',
	// but 'issue' asset directly to others.
	if addrHasBalance == 1 {
		addrAsset = &addr.Asset{
			Address:             adminAddr,
			AssetID:             assetID,
			Balance:             adminBalance,
			Transactions:        0,
			LastTransactionTime: 0,
		}
	}

	return nep5, addrAsset, uint(minHeight), true
}

func createNep5BalanceSCSB(scriptHash []byte, addrBytes []byte) string {
	if len(addrBytes) == 0 {
		return ""
	}

	return createSCSB(scriptHash, "balanceOf", [][]byte{addrBytes})
}

func createSCSB(scriptHash []byte, method string, params [][]byte) string {
	scsb := smartcontract.ScriptBuilder{
		ScriptHash: scriptHash,
		Method:     method,
		Params:     params,
	}

	return scsb.GetScript()
}

func queryCallerBalance(txBlockIndex uint, blockTime uint64, scriptHash []byte, callerAddrBytes []byte) (*big.Float, bool) {
	assetID := util.GetAssetIDFromScriptHash(scriptHash)
	callerAddr := util.GetAddressFromScriptHash(callerAddrBytes)

	// Query from cache.
	if cached, ok := cache.GetAddrAsset(callerAddr, assetID); ok {
		// If cache valid.
		if cached.BlockIndex > txBlockIndex {
			return cached.Balance, true
		}
	}

	decimals, ok := nep5AssetDecimals[assetID]
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
	if callerBalance.Cmp(maxVal) > 0 {
		callerBalance = big.NewFloat(0)
	}

	return callerBalance, true
}

func queryBalances(txBlockIndex uint, scriptHash []byte, assetID string, addrBytesList [][]byte) ([]*big.Float, bool) {
	// Check if this is a valid assetID.
	if _, ok := nep5AssetDecimals[assetID]; !ok {
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
			scsb += createNep5BalanceSCSB(scriptHash, addrBytes)
		}
	}

	minHeight := rpc.BestHeight.Get()
	result := rpc.SmartContractRPCCall(minHeight, scsb)

	// If this nep5 asset is broken(for example forgot to check 'need storage').
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
		balances[i] = getReadableValue(assetID, balance)
		if balances[i].Cmp(maxVal) > 0 {
			balances[i] = big.NewFloat(0)
		}
		idx++
	}

	return balances, true
}

func queryNep5TotalSupply(txBlockIndex uint, blockTime uint64, scriptHash []byte) (*big.Float, bool) {
	assetID := util.GetAssetIDFromScriptHash(scriptHash)

	decimals, ok := nep5AssetDecimals[assetID]
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

func getReadableValue(assetID string, balance *big.Float) *big.Float {
	zeroValue := big.NewFloat(0)
	if balance.Cmp(zeroValue) == 0 {
		return zeroValue
	}

	decimals, ok := nep5AssetDecimals[assetID]
	if !ok {
		panic("Failed to get decimals of nep5 asset: " + assetID)
	}

	return new(big.Float).SetPrec(256).Quo(balance, big.NewFloat(math.Pow10(int(decimals))))
}

func showNep5Progress(txPk uint) {
	if maxNep5PK == 0 || Nep5MaxPkShouldRefresh {
		Nep5MaxPkShouldRefresh = false
		maxNep5PK = db.GetMaxNonEmptyScriptTxPk()
	}

	now := time.Now()
	if nProgress.LastOutputTime == (time.Time{}) {
		nProgress.LastOutputTime = now
	}
	if txPk < maxNep5PK && now.Sub(nProgress.LastOutputTime) < time.Second {
		return
	}

	GetEstimatedRemainingTime(int64(txPk), int64(maxNep5PK), &nProgress)
	if nProgress.Percentage.Cmp(big.NewFloat(100)) == 0 &&
		bProgress.Finished {
		nProgress.Finished = true
	}

	log.Printf("%sProgress of nep5: %d/%d, %.4f%%\n",
		nProgress.RemainingTimeStr,
		txPk,
		maxNep5PK,
		nProgress.Percentage)
	nProgress.LastOutputTime = now

	// Send mail if fully synced
	if nProgress.Finished && !nProgress.MailSent {
		nProgress.MailSent = true

		// If sync lasts shortly, do not send mail
		if time.Since(nProgress.InitTime) < time.Minute*5 {
			return
		}

		msg := fmt.Sprintf("Init time: %v\nEnd Time: %v\n", nProgress.InitTime, time.Now())
		mail.SendNotify("NEP5 TX Fully Synced", msg)
	}
}

func getMinHeight(blockHeight uint) int {
	bestHeight := rpc.BestHeight.Get()
	if bestHeight > int(blockHeight) {
		return bestHeight
	}

	return int(blockHeight)
}
