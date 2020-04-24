package tasks

import (
	"squirrel/db"
	"squirrel/mail"
	"time"
)

func startUpdateCounterTask() {
	go insertNep5AddrTxRecord()
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
