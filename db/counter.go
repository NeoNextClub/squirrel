package db

import (
	"database/sql"
	"fmt"
	"squirrel/tx"
)

// Counter db model.
type Counter struct {
	ID                 uint
	LastBlockIndex     int
	LastTxPk           uint
	LastAssetTxPk      uint
	LastTxPkForNep5    uint
	AppLogIdx          int
	LastTxPkForNft     uint
	NftAppLogIdx       int
	LastTxPkForSC      uint
	Nep5TxPkForAddrTx  uint
	NftTxPkForAddrTx   uint
	LastTxPkGasBalacne uint
	CntAddr            uint
	CntTxReg           uint
	CntTxMiner         uint
	CntTxIssue         uint
	CntTxInvocation    uint
	CntTxContract      uint
	CntTxClaim         uint
	CntTxPublish       uint
	CntTxEnrollment    uint
}

// GetLastHeight returns the highest block index stored in database.
func GetLastHeight() int {
	counter := getCounterInstance()
	return counter.LastBlockIndex
}

func initCounterInstance() Counter {
	c := Counter{
		ID:                 1,
		LastBlockIndex:     -1,
		LastTxPk:           0,
		LastAssetTxPk:      0,
		LastTxPkForNep5:    0,
		AppLogIdx:          -1,
		LastTxPkForNft:     0,
		NftAppLogIdx:       -1,
		LastTxPkForSC:      0,
		Nep5TxPkForAddrTx:  0,
		NftTxPkForAddrTx:   0,
		LastTxPkGasBalacne: 0,
		CntAddr:            0,
		CntTxReg:           0,
		CntTxMiner:         0,
		CntTxIssue:         0,
		CntTxInvocation:    0,
		CntTxContract:      0,
		CntTxClaim:         0,
		CntTxPublish:       0,
		CntTxEnrollment:    0,
	}
	const query = "INSERT INTO `counter` (`id`, `last_block_index`, `last_tx_pk`, `last_asset_tx_pk`, `last_tx_pk_for_nep5`, `app_log_idx`, `last_tx_pk_for_nft`, `nft_app_log_idx`, `last_tx_pk_for_sc`, `nep5_tx_pk_for_addr_tx`, `nft_tx_pk_for_addr_tx`, `last_tx_pk_gas_balance`, `cnt_addr`, `cnt_tx_reg`, `cnt_tx_miner`, `cnt_tx_issue`, `cnt_tx_invocation`, `cnt_tx_contract`, `cnt_tx_claim`, `cnt_tx_publish`, `cnt_tx_enrollment`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	_, err := db.Exec(query,
		c.ID,
		c.LastBlockIndex,
		c.LastTxPk,
		c.LastAssetTxPk,
		c.LastTxPkForNep5,
		c.AppLogIdx,
		c.LastTxPkForNft,
		c.NftAppLogIdx,
		c.LastTxPkForSC,
		c.Nep5TxPkForAddrTx,
		c.NftTxPkForAddrTx,
		c.LastTxPkGasBalacne,
		c.CntAddr,
		c.CntTxReg,
		c.CntTxMiner,
		c.CntTxIssue,
		c.CntTxInvocation,
		c.CntTxContract,
		c.CntTxClaim,
		c.CntTxPublish,
		c.CntTxEnrollment,
	)
	if err != nil {
		panic(err)
	}

	return c
}

func getCounterInstance() Counter {
	const query = "SELECT `id`, `last_block_index`, `last_tx_pk`, `last_asset_tx_pk`, `last_tx_pk_for_nep5`, `app_log_idx`, `last_tx_pk_for_nft`, `nft_app_log_idx`, `last_tx_pk_for_sc`, `nep5_tx_pk_for_addr_tx`, `nft_tx_pk_for_addr_tx`, `last_tx_pk_gas_balance` FROM `counter` WHERE `id` = 1 LIMIT 1"

	var counter Counter
	err := db.QueryRow(query).Scan(
		&counter.ID,
		&counter.LastBlockIndex,
		&counter.LastTxPk,
		&counter.LastAssetTxPk,
		&counter.LastTxPkForNep5,
		&counter.AppLogIdx,
		&counter.LastTxPkForNft,
		&counter.NftAppLogIdx,
		&counter.LastTxPkForSC,
		&counter.Nep5TxPkForAddrTx,
		&counter.NftTxPkForAddrTx,
		&counter.LastTxPkGasBalacne,
	)
	switch err {
	case sql.ErrNoRows:
		return initCounterInstance()
	case nil:
		return counter
	default:
		reconnect()
		return getCounterInstance()
	}
}

// GetLastTxPkCounter returns the last resolved pk of transaction in counter.
func GetLastTxPkCounter() uint {
	counter := getCounterInstance()
	return counter.LastTxPk
}

// GetLastAssetTxPkCounter returns the last resolved pk of asset transaction in counter.
func GetLastAssetTxPkCounter() uint {
	counter := getCounterInstance()
	return counter.LastAssetTxPk
}

// GetLastTxPkForNep5 returns counter info of last processed nep5 transactions.
func GetLastTxPkForNep5() (uint, int) {
	counter := getCounterInstance()
	return counter.LastTxPkForNep5, counter.AppLogIdx
}

// GetLastTxPkForNft returns counter info of last processed nft transactions.
func GetLastTxPkForNft() (uint, int) {
	counter := getCounterInstance()
	return counter.LastTxPkForNft, counter.NftAppLogIdx
}

// GetLastTxPkForSC returns counter info of last processed sc transactions.
func GetLastTxPkForSC() uint {
	counter := getCounterInstance()
	return counter.LastTxPkForSC
}

// GetLastTxPkForGasBalance returns the last resolved pk of gas balance task.
func GetLastTxPkForGasBalance() uint {
	counter := getCounterInstance()
	return counter.LastTxPkGasBalacne
}

// GetNep5TxPkForAddrTx returns last pk of handled nep5 tx records.
func GetNep5TxPkForAddrTx() uint {
	counter := getCounterInstance()
	return counter.Nep5TxPkForAddrTx
}

// GetNftTxPkForAddrTx returns last pk of handled nft tx records.
func GetNftTxPkForAddrTx() uint {
	counter := getCounterInstance()
	return counter.NftTxPkForAddrTx
}

// UpdateLastTxPk updates last pk of processed transaction.
func UpdateLastTxPk(txPk uint) error {
	const updateCounterSQL = "UPDATE `counter` SET `last_tx_pk` = ? WHERE `id` = 1 LIMIT 1"
	_, err := db.Exec(updateCounterSQL, txPk)
	return err
}

// UpdateLastTxPkForNep5 updates counter info of last processed nep5 transactions.
func UpdateLastTxPkForNep5(currentTxPk uint, applogIdx int) error {
	const updateCounterSQL = "UPDATE `counter` SET `last_tx_pk_for_nep5` = ?, `app_log_idx` = ? WHERE `id` = 1 LIMIT 1"
	_, err := db.Exec(updateCounterSQL, currentTxPk, applogIdx)
	return err
}

// UpdateLastTxPkForNft updates counter info of last processed nft transactions.
func UpdateLastTxPkForNft(currentTxPk uint, applogIdx int) error {
	const updateCounterSQL = "UPDATE `counter` SET `last_tx_pk_for_nft` = ?, `nft_app_log_idx` = ? WHERE `id` = 1 LIMIT 1"
	_, err := db.Exec(updateCounterSQL, currentTxPk, applogIdx)
	return err
}

// UpdateLastTxPkForSC updates counter info of last processed sc transactions.
func UpdateLastTxPkForSC(currentTxPk uint) error {
	const updateCounterSQL = "UPDATE `counter` SET `last_tx_pk_for_sc` = ? WHERE `id` = 1 LIMIT 1"
	_, err := db.Exec(updateCounterSQL, currentTxPk)
	return err
}

func updateCounter(tx *sql.Tx, key string, value int64) error {
	sql := fmt.Sprintf("UPDATE `counter` SET %s = %d WHERE `id`=1", key, value)

	_, err := tx.Exec(sql)
	return err
}

func updateNep5Counter(tx *sql.Tx, lastTxPkForNep5 uint, appLogIdx int) error {
	const sql = "UPDATE `counter` SET `last_tx_pk_for_nep5` = ?, `app_log_idx` = ? WHERE `id` = 1 LIMIT 1"
	_, err := tx.Exec(sql, lastTxPkForNep5, appLogIdx)
	return err
}

func updateNftCounter(tx *sql.Tx, lastTxPkForNft uint, appLogIdx int) error {
	const sql = "UPDATE `counter` SET `last_tx_pk_for_nft` = ?, `nft_app_log_idx` = ? WHERE `id` = 1 LIMIT 1"
	_, err := tx.Exec(sql, lastTxPkForNft, appLogIdx)
	return err
}

// UpdateNep5TxPkForAddrTx updates last pk of handled nep5 tx records.
func UpdateNep5TxPkForAddrTx(tx *sql.Tx, pk uint) error {
	const query = "UPDATE `counter` SET `nep5_tx_pk_for_addr_tx` = ? WHERE `id` = 1 LIMIT 1"
	_, err := tx.Exec(query, pk)
	return err
}

// UpdateNftTxPkForAddrTx updates last pk of handled nft tx records.
func UpdateNftTxPkForAddrTx(tx *sql.Tx, pk uint) error {
	const query = "UPDATE `counter` SET `nft_tx_pk_for_addr_tx` = ? WHERE `id` = 1 LIMIT 1"
	_, err := tx.Exec(query, pk)
	return err
}

func updateTxCounter(trans *sql.Tx, txType int, cnt int) error {
	query := ""

	switch txType {
	case tx.RegisterTransaction:
		query = "UPDATE `counter` SET `cnt_tx_reg` = `cnt_tx_reg` + ? WHERE `id` = 1 LIMIT 1"
	case tx.MinerTransaction:
		query = "UPDATE `counter` SET `cnt_tx_miner` = `cnt_tx_miner` + ? WHERE `id` = 1 LIMIT 1"
	case tx.IssueTransaction:
		query = "UPDATE `counter` SET `cnt_tx_issue` = `cnt_tx_issue` + ? WHERE `id` = 1 LIMIT 1"
	case tx.InvocationTransaction:
		query = "UPDATE `counter` SET `cnt_tx_invocation` = `cnt_tx_invocation` + ? WHERE `id` = 1 LIMIT 1"
	case tx.ContractTransaction:
		query = "UPDATE `counter` SET `cnt_tx_contract` = `cnt_tx_contract` + ? WHERE `id` = 1 LIMIT 1"
	case tx.ClaimTransaction:
		query = "UPDATE `counter` SET `cnt_tx_claim` = `cnt_tx_claim` + ? WHERE `id` = 1 LIMIT 1"
	case tx.PublishTransaction:
		query = "UPDATE `counter` SET `cnt_tx_publish` = `cnt_tx_publish` + ? WHERE `id` = 1 LIMIT 1"
	case tx.EnrollmentTransaction:
		query = "UPDATE `counter` SET `cnt_tx_enrollment` = `cnt_tx_enrollment` + ? WHERE `id` = 1 LIMIT 1"
	default:
		panic("Unknown transaction type when updating counter of tx types")
	}

	_, err := trans.Exec(query, cnt)
	return err
}

func incrAddrCounter(trans *sql.Tx, delta int) error {
	query := "UPDATE `counter` SET `cnt_addr` = `cnt_addr` + ? WHERE `id` = 1 LIMIT 1"
	_, err := trans.Exec(query, delta)
	return err
}
