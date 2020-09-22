package tasks

import (
	"fmt"
	"math/big"
	"squirrel/asset"
	"squirrel/db"
	"squirrel/log"
	"squirrel/mail"
	"time"
)

const gasBalanceChainSize = 5000

var (
	gasMaxPkShouldRefresh bool
	gasProgress           = Progress{}
	maxTxPkForGas         uint
)

func startGasBalanceTask() {
	gasBalanceChan := make(chan txInfo, gasBalanceChainSize)
	nextPK := db.GetLastTxPkForGasBalance() + 1

	go fetchTx(gasBalanceChan, nextPK)
	go handleTxGASBalance(gasBalanceChan)
}

func handleTxGASBalance(gasBalanceChan <-chan txInfo) {
	defer mail.AlertIfErr()

	for info := range gasBalanceChan {
		gasChangeMap := getGASChange(info)

		if len(gasChangeMap) == 0 {
			continue
		}

		date := time.Unix(int64(info.tx.BlockTime), 0)
		err := db.ApplyGASAssetChange(info.tx, date.Format("2006-01-02"), gasChangeMap)
		if err != nil {
			panic(err)
		}

		showGasDateBalanceProgress(info.tx.ID)
	}
}

func getGASChange(info txInfo) map[string]*big.Float {
	vins := info.vins
	vouts := info.vouts

	if len(vins) == 0 && len(vouts) == 0 {
		return nil
	}

	gasMap := make(map[string]*big.Float)

	for _, vin := range vins {
		vinVout, err := db.GetVout(vin.TxID, vin.Vout)
		if err != nil {
			panic(err)
		}

		if vinVout.AssetID == asset.GASAssetID {
			negAmount := new(big.Float).SetPrec(256).Neg(vinVout.Value)
			updateMapValue(gasMap, vinVout.Address, negAmount)
		}
	}

	for _, vout := range vouts {
		if vout.AssetID == asset.GASAssetID {
			updateMapValue(gasMap, vout.Address, vout.Value)
		}
	}

	return gasMap
}

func updateMapValue(mp map[string]*big.Float, key string, offset *big.Float) {
	if mp == nil {
		mp = make(map[string]*big.Float)
	}

	value, ok := mp[key]
	if ok {
		mp[key] = new(big.Float).SetPrec(256).Add(value, offset)
		return
	}

	mp[key] = new(big.Float).SetPrec(256).Set(offset)
}

func showGasDateBalanceProgress(currentTxPK uint) {
	if maxTxPkForGas == 0 || gasMaxPkShouldRefresh {
		gasMaxPkShouldRefresh = false
		maxTxPkForGas = db.GetHighestTxPk()
	}

	now := time.Now()
	if gasProgress.LastOutputTime == (time.Time{}) {
		gasProgress.LastOutputTime = now
	}
	if currentTxPK < maxTxPkForGas && now.Sub(gasProgress.LastOutputTime) < time.Second {
		return
	}

	GetEstimatedRemainingTime(int64(currentTxPK), int64(maxTxPkForGas), &gasProgress)
	if gasProgress.Percentage.Cmp(big.NewFloat(100)) == 0 &&
		bProgress.Finished {
		gasProgress.Finished = true
	}

	log.Printf("%sProgress of Addr-Date-Gas: %d/%d, %.4f%%\n",
		gasProgress.RemainingTimeStr,
		currentTxPK,
		maxTxPkForGas,
		gasProgress.Percentage)

	gasProgress.LastOutputTime = now

	// Send mail if fully synced.
	if gasProgress.Finished && !gasProgress.MailSent {
		gasProgress.MailSent = true

		// If sync lasts shortly, do not send mail.
		if time.Since(gasProgress.InitTime) < time.Minute*5 {
			return
		}

		msg := fmt.Sprintf("Init time: %v\nEnd Time: %v\n", gasProgress.InitTime, time.Now())
		mail.SendNotify("Addr-Date-Gas Fully Synced", msg)
	}
}
