/*
To restart this task from beginning, execute the following sqls:

UPDATE `counter` SET `last_tx_pk_for_sc` = 0 WHERE `id` = 1 LIMIT 1;
TRUNCATE TABLE `smartcontract_info`;

*/

package tasks

import (
	"fmt"
	"math/big"
	"squirrel/db"
	"squirrel/log"
	"squirrel/mail"
	"squirrel/nep5"
	"squirrel/smartcontract"
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
	scriptInfoList []scriptInfo
	txPK           uint
}

type scriptInfo struct {
	txID   string
	script string
}

func startSCTask() {
	scTxChan := make(chan scStore, scChanSize)

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
			continue
		}

		nextTxPK = txs[len(txs)-1].ID + 1

		scriptInfoList := []scriptInfo{}
		for _, tx := range txs {
			scriptInfoList = append(scriptInfoList, scriptInfo{
				txID:   tx.TxID,
				script: tx.Script,
			})
		}

		scTxChan <- scStore{
			scriptInfoList: scriptInfoList,
			txPK:           txs[len(txs)-1].ID + 1,
		}
	}
}

func handleScTx(scTxChan <-chan scStore) {
	defer mail.AlertIfErr()

	for scInfo := range scTxChan {
		scRegInfos := filterSC(scInfo.scriptInfoList)
		if len(scRegInfos) > 0 {
			db.InsertSCInfos(scRegInfos, scInfo.txPK)
		}

		showSCProgress(scInfo.txPK)
	}
}

func filterSC(list []scriptInfo) []*nep5.RegInfo {
	result := []*nep5.RegInfo{}

	for _, info := range list {
		if !strings.HasSuffix(info.script, "4e656f2e436f6e74726163742e437265617465") &&
			!isNep5RegistrationTx(info.script) &&
			!isNep5MigrateTx(info.script) {
			continue
		}

		opCodeDataStack := smartcontract.ReadScript(info.script)
		if opCodeDataStack == nil || len(*opCodeDataStack) == 0 {
			continue
		}

		regInfo, ok := nep5.GetNep5RegInfo(info.txID, opCodeDataStack.Copy())
		if !ok {
			continue
		}

		result = append(result, regInfo)
	}

	return result
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
