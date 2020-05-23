package tasks

import (
	"squirrel/db"
	"squirrel/mail"
	"time"
)

func startUpdateCounterTask() {
	go insertNep5AddrTxRecord()
	go insertNftAddrTxRecord()
}

func insertNep5AddrTxRecord() {
	defer mail.AlertIfErr()

	lastPk := db.GetNep5TxPkForAddrTx()

	for {
		Nep5TxRecs, err := db.GetNep5TxRecords(lastPk, 1000)
		if err != nil {
			panic(err)
		}

		if len(Nep5TxRecs) > 0 {
			lastPk = Nep5TxRecs[len(Nep5TxRecs)-1].ID
			err = db.InsertNep5AddrTxRec(Nep5TxRecs, lastPk)
			if err != nil {
				panic(err)
			}

			time.Sleep(time.Millisecond * 10)
			continue
		}

		time.Sleep(time.Second)
	}
}

func insertNftAddrTxRecord() {
	defer mail.AlertIfErr()

	lastPk := db.GetNftTxPkForAddrTx()

	for {
		NftTxRecs, err := db.GetNftTxRecords(lastPk, 1000)
		if err != nil {
			panic(err)
		}

		if len(NftTxRecs) > 0 {
			lastPk = NftTxRecs[len(NftTxRecs)-1].ID
			err = db.InsertNftAddrTxRec(NftTxRecs, lastPk)
			if err != nil {
				panic(err)
			}

			time.Sleep(time.Millisecond * 10)
			continue
		}

		time.Sleep(time.Second)
	}
}
