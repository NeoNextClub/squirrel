package tasks

import (
	"fmt"
	"math/big"
	"squirrel/db"
	"squirrel/log"
	"squirrel/mail"
	"squirrel/nep5"
	"squirrel/smartcontract"
	"squirrel/util"
	"strings"
	"time"
)

const scChanSize = 5000

var (
	scMaxPkShouldRefresh bool

	scProgress = Progress{}
	maxScPK    uint
)

type scStore struct {
	txPK      uint
	scriptHex string
}

func startSCTask() {
	scTxChan := make(chan scStore)

	lastPk := db.GetLastTxPkForSC()

	go fetchSCTx(scTxChan, lastPk)
	go handleScTx(scTxChan)
}

func fetchSCTx(scTxChan chan<- scStore, lastPk uint) {
	defer mail.AlertIfErr()

	nextTxPK := lastPk + 1

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
		}

		nextTxPK = txs[len(txs)-1].ID + 1
		for _, tx := range txs {
			scTxChan <- scStore{
				txPK:      tx.ID,
				scriptHex: tx.Script,
			}
		}
	}
}

func handleScTx(scTxChan <-chan scStore) {
	defer mail.AlertIfErr()

	for scInfo := range scTxChan {
		if !handleSC(scInfo) {
			handleSCCounterStore(scInfo.txPK)
			continue
		}

		showSCProgress(scInfo.txPK)
	}
}

func handleSC(sc scStore) bool {
	if !strings.HasSuffix(sc.scriptHex, "4e656f2e436f6e74726163742e437265617465") {
		return false
	}

	opCodeDataStack := smartcontract.ReadScript(sc.scriptHex)
	if opCodeDataStack == nil || len(*opCodeDataStack) == 0 {
		return false
	}

	scriptBytes, regInfo, ok := nep5.GetNep5RegInfo(opCodeDataStack.Copy())
	if !ok {
		return false
	}

	scriptHashBytes := util.GetScriptHash(scriptBytes)
	scriptHash := util.GetAssetIDFromScriptHash(scriptHashBytes)

	err := db.InsertSCInfo(scriptHash, regInfo)
	if err != nil {
		panic(err)
	}

	return true
}

func handleSCCounterStore(txPK uint) {
	err := db.UpdateLastTxPkForSC(txPK)
	if err != nil {
		panic(err)
	}

	showSCProgress(txPK)
}

func showSCProgress(txPk uint) {
	if maxScPK == 0 || scMaxPkShouldRefresh {
		scMaxPkShouldRefresh = false
		maxScPK = db.GetMaxNonEmptyScriptTxPk()
	}

	now := time.Now()
	if scProgress.LastOutputTime == (time.Time{}) {
		scProgress.LastOutputTime = now
	}
	if txPk < maxScPK && now.Sub(scProgress.LastOutputTime) < time.Second {
		return
	}

	GetEstimatedRemainingTime(int64(txPk), int64(maxScPK), &scProgress)
	if scProgress.Percentage.Cmp(big.NewFloat(100)) == 0 &&
		bProgress.Finished {
		scProgress.Finished = true
	}

	log.Printf("%sProgress of smart contract cnt: %d/%d, %.4f%%\n",
		scProgress.RemainingTimeStr,
		txPk,
		maxScPK,
		scProgress.Percentage)
	scProgress.LastOutputTime = now

	// Send mail if fully synced
	if scProgress.Finished && !scProgress.MailSent {
		scProgress.MailSent = true

		// If sync lasts shortly, do not send mail
		if time.Since(scProgress.InitTime) < time.Minute*5 {
			return
		}

		msg := fmt.Sprintf("Init time: %v\nEnd Time: %v\n", scProgress.InitTime, time.Now())
		mail.SendNotify("NEP5 TX Fully Synced", msg)
	}
}
