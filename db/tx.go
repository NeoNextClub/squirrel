package db

import (
	"database/sql"
	"fmt"
	"math/big"
	"sort"
	"squirrel/asset"
	"squirrel/cache"
	"squirrel/tx"
	"squirrel/util"
	"strings"
)

// GetTxs returns transactions of given tx pk range.
func GetTxs(txPk uint, limit int, txType string) []*tx.Transaction {
	txSQL := "SELECT `id`, `block_index`, `block_time`, `txid`, `size`, `type`, `version`, `sys_fee`, `net_fee`, `nonce`, `script`, `gas` FROM `tx` WHERE `id` >= ?"

	if txType != "" {
		txSQL += fmt.Sprintf(" AND `type` = %s", txType)
	}

	txSQL += " AND (EXISTS(SELECT `id` FROM `tx_vin` WHERE `from`=`tx`.`txid` LIMIT 1) OR EXISTS (SELECT `id` FROM `tx_vout` WHERE `txid`=`tx`.`txid` LIMIT 1)) ORDER BY ID ASC LIMIT ?"

	rows, err := wrappedQuery(txSQL, txPk, limit)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	result := []*tx.Transaction{}

	for rows.Next() {
		var t tx.Transaction
		sysFeeStr := ""
		netFeeStr := ""
		gasStr := ""

		err := rows.Scan(
			&t.ID,
			&t.BlockIndex,
			&t.BlockTime,
			&t.TxID,
			&t.Size,
			&t.Type,
			&t.Version,
			&sysFeeStr,
			&netFeeStr,
			&t.Nonce,
			&t.Script,
			&gasStr,
		)

		if err != nil {
			panic(err)
		}

		t.SysFee = util.StrToBigFloat(sysFeeStr)
		t.NetFee = util.StrToBigFloat(netFeeStr)
		t.Gas = util.StrToBigFloat(gasStr)

		result = append(result, &t)
	}

	return result
}

// GetTxAttrs returns attributes of a transaction.
func GetTxAttrs(txID string) []*tx.TransactionAttribute {
	query := []string{
		"SELECT `id`, `txid`, `usage`, `data`",
		"FROM `tx_attr`",
		fmt.Sprintf("WHERE `txid` = '%s'", txID),
	}

	rows, err := wrappedQuery(strings.Join(query, " "))
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	attrs := []*tx.TransactionAttribute{}

	for rows.Next() {
		var attr tx.TransactionAttribute

		err := rows.Scan(
			&attr.ID,
			&attr.TxID,
			&attr.Usage,
			&attr.Data,
		)
		if err != nil {
			panic(err)
		}

		attrs = append(attrs, &attr)
	}

	return attrs
}

// GetVinVout returns correspond vouts of vins.
func GetVinVout(txIDs []string) (map[string][]*tx.TransactionVin, map[string][]*tx.TransactionVout, error) {
	vinMap, err := GetVins(txIDs)
	if err != nil {
		return nil, nil, err
	}

	voutMap, err := GetVouts(txIDs)
	if err != nil {
		return nil, nil, err
	}

	return vinMap, voutMap, nil
}

// GetVins returns all vins of the given txID.
func GetVins(txIDs []string) (map[string][]*tx.TransactionVin, error) {
	query := "SELECT `from`, `txid`, `vout` FROM `tx_vin` WHERE `from` IN ('"
	query += strings.Join(txIDs, "', '")
	query += "')"

	vinMap := make(map[string][]*tx.TransactionVin)

	rows, err := wrappedQuery(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		vin := new(tx.TransactionVin)
		err := rows.Scan(
			// &vin.ID,
			&vin.From,
			&vin.TxID,
			&vin.Vout,
		)
		if err != nil {
			panic(err)
		}

		vinMap[vin.From] = append(vinMap[vin.From], vin)
	}

	return vinMap, nil
}

// GetVouts returns all vins of the given txID.
func GetVouts(txIDs []string) (map[string][]*tx.TransactionVout, error) {
	query := "SELECT `txid`, `n`, `asset_id`, `value`, `address` FROM `tx_vout` WHERE `txid` IN ('"
	query += strings.Join(txIDs, "', '")
	query += "')"

	voutMap := make(map[string][]*tx.TransactionVout)

	rows, err := wrappedQuery(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		vout := new(tx.TransactionVout)
		valueStr := ""
		err := rows.Scan(
			// &vout.ID,
			&vout.TxID,
			&vout.N,
			&vout.AssetID,
			&valueStr,
			&vout.Address,
		)
		if err != nil {
			panic(err)
		}

		vout.Value = util.StrToBigFloat(valueStr)

		voutMap[vout.TxID] = append(voutMap[vout.TxID], vout)
	}
	return voutMap, nil
}

func handleVins(blockIndex uint, tx *sql.Tx, vins []*tx.TransactionVin, cachedVinVouts *[]*tx.TransactionVout) error {
	for _, vin := range vins {
		const disableUTXOSQL = "UPDATE `utxo` SET `used_in_tx` = ? WHERE `txid` = ? AND `n` = ? LIMIT 1"
		_, err := tx.Exec(disableUTXOSQL, vin.From, vin.TxID, vin.Vout)
		if err != nil {
			return err
		}

		vinVout, err := GetVout(vin.TxID, vin.Vout)
		if err != nil {
			return err
		}
		*cachedVinVouts = append(*cachedVinVouts, vinVout)

		// 'last_transaction_time' will be updated later.
		if addrAssetCache, ok := cache.GetAddrAsset(vinVout.Address, vinVout.AssetID); ok {
			// This subtraction will always be executed.
			addrAssetCache.SubtractBalance(vinVout.Value, blockIndex)
		}
		reduceAddrAssetSQL := fmt.Sprintf("UPDATE `addr_asset` SET `balance` = `balance` - %.8f WHERE `address` = '%s' AND `asset_id` = '%s' LIMIT 1", vinVout.Value, vinVout.Address, vinVout.AssetID)
		_, err = tx.Exec(reduceAddrAssetSQL)
		if err != nil {
			return err
		}
	}

	return nil
}

func handleVouts(blockIndex uint, blockTime uint64, tx *sql.Tx, vouts []*tx.TransactionVout) error {
	for _, vout := range vouts {
		insertUTXOQuery := fmt.Sprintf("INSERT INTO `utxo` (`address`, `txid`, `n`, `asset_id`, `value`, `used_in_tx`) VALUES ('%s', '%s', %d, '%s', %.8f, null)", vout.Address, vout.TxID, vout.N, vout.AssetID, vout.Value)
		if _, err := tx.Exec(insertUTXOQuery); err != nil {
			return err
		}

		cachedAddr, _ := cache.GetAddrOrCreate(vout.Address, blockTime)
		addrAssetCache, created := cachedAddr.GetAddrAssetOrCreate(vout.AssetID, vout.Value)

		if created {
			// Transactions counter and last transaction time will be updated later, currently set its initial value to 0.
			insertAddrAssetQuery := fmt.Sprintf("INSERT INTO `addr_asset` (`address`, `asset_id`, `balance`, `transactions`, `last_transaction_time`) VALUES ('%s', '%s', %.8f, %d, %d)", vout.Address, vout.AssetID, vout.Value, 0, 0)
			if _, err := tx.Exec(insertAddrAssetQuery); err != nil {
				return err
			}
			// Increase asset addresses count.
			incrAssetAddrCount := fmt.Sprintf("UPDATE `asset` SET `addresses` = `addresses` + 1 WHERE `asset_id` = '%s' LIMIT 1", vout.AssetID)
			if _, err := tx.Exec(incrAssetAddrCount); err != nil {
				return err
			}
		} else {
			addrAssetCache.AddBalance(vout.Value, blockIndex)
			// 'last_transaction_time' will be updated later.
			incrAddrAsset := fmt.Sprintf("UPDATE `addr_asset` SET `balance` = `balance` + %.8f WHERE `address` = '%s' AND `asset_id` = '%s' LIMIT 1", vout.Value, vout.Address, vout.AssetID)
			if _, err := tx.Exec(incrAddrAsset); err != nil {
				return err
			}
		}
	}

	return nil
}

// RecordAddrAssetIDTx records {address, asset_id, txid}.
func RecordAddrAssetIDTx(records []tx.AddrAssetIDTx, txPK int64) error {
	if len(records) == 0 {
		return nil
	}

	return transact(func(trans *sql.Tx) error {
		piece := 100

		for start := 0; start < len(records); start += piece {
			query := "INSERT INTO `asset_tx` (`address`, `asset_id`, `txid`) VALUES "
			for i := start; i < start+piece; i++ {
				if i >= len(records) {
					break
				}
				query += fmt.Sprintf("('%s', '%s', '%s'), ", records[i].Address, records[i].AssetID, records[i].TxID)
			}

			if !strings.HasSuffix(query, ", ") {
				break
			}

			query = query[:len(query)-2]
			_, err := trans.Exec(query)
			if err != nil {
				return err
			}
		}

		err := updateCounter(trans, "last_asset_tx_pk", txPK)
		if err != nil {
			return err
		}

		return nil
	})
}

// ApplyVinsVouts process transaction and update related db table info.
func ApplyVinsVouts(t *tx.Transaction, vins []*tx.TransactionVin, vouts []*tx.TransactionVout) error {
	return transact(func(trans *sql.Tx) error {
		cachedVinVouts := []*tx.TransactionVout{}

		if err := handleVins(t.BlockIndex, trans, vins, &cachedVinVouts); err != nil {
			return err
		}

		assetIDs, addrAssetPair := countTxInfo(cachedVinVouts, vouts)

		// Sort keys of addrAssetPair to avoid potential deadlock.
		var addrs []string
		for k := range addrAssetPair {
			addrs = append(addrs, k)
		}
		// Sort address to avoid potential deadlock.
		sort.Strings(addrs)

		createdAddrCnt := 0

		for _, addr := range addrs {
			// Update address table.
			created, err := updateAddrInfo(trans, t.BlockTime, t.TxID, addr, asset.ASSET)
			if err != nil {
				return err
			}

			if created {
				createdAddrCnt++
			}
		}

		if err := handleVouts(t.BlockIndex, t.BlockTime, trans, vouts); err != nil {
			return err
		}

		if t.Type == "ClaimTransaction" {
			if err := handleClaimTx(trans, vouts); err != nil {
				return err
			}
		}
		if t.Type == "IssueTransaction" {
			if err := handleIssueTx(trans, vouts); err != nil {
				return err
			}
		}

		if err := updateTxInfo(trans, t.BlockTime, t.TxID, addrs, assetIDs, addrAssetPair); err != nil {
			return err
		}

		if createdAddrCnt > 0 {
			if err := incrAddrCounter(trans, createdAddrCnt); err != nil {
				return err
			}
		}

		if err := updateCounter(trans, "last_tx_pk", int64(t.ID)); err != nil {
			return err
		}

		return nil
	})
}

func handleClaimTx(tx *sql.Tx, vouts []*tx.TransactionVout) error {
	gas := big.NewFloat(0)

	for _, vout := range vouts {
		if vout.AssetID == asset.GASAssetID {
			gas = new(big.Float).SetPrec(256).Add(gas, vout.Value)
		}
	}

	query := fmt.Sprintf("UPDATE `asset` SET `available` = `available` + %.8f WHERE `asset_id` = '%s' LIMIT 1", gas, asset.GASAssetID)
	if _, err := tx.Exec(query); err != nil {
		return err
	}

	return nil
}

func handleIssueTx(tx *sql.Tx, vouts []*tx.TransactionVout) error {
	issued := make(map[string]*big.Float)

	for _, vout := range vouts {
		if vout.AssetID != asset.GASAssetID {
			if _, ok := issued[vout.AssetID]; !ok {
				issued[vout.AssetID] = vout.Value
			} else {
				issued[vout.AssetID] = new(big.Float).SetPrec(256).Add(issued[vout.AssetID], vout.Value)
			}
		}
	}
	for assetID, increment := range issued {
		query := fmt.Sprintf("UPDATE `asset` SET `available` = `available` + %.8f WHERE `asset_id` = '%s' LIMIT 1", increment, assetID)
		if _, err := tx.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

func countTxInfo(cachedVinVouts []*tx.TransactionVout, vouts []*tx.TransactionVout) (map[string]bool, map[string]map[string]bool) {
	// [addr, [assetID, bool]]
	addrAssetPair := make(map[string]map[string]bool)
	assetIDs := make(map[string]bool)

	for _, vinVout := range cachedVinVouts {
		assetIDs[vinVout.AssetID] = true
		if _, ok := addrAssetPair[vinVout.Address]; !ok {
			addrAssetPair[vinVout.Address] = make(map[string]bool)
		}
		addrAssetPair[vinVout.Address][vinVout.AssetID] = true
	}
	for _, vout := range vouts {
		assetIDs[vout.AssetID] = true
		if _, ok := addrAssetPair[vout.Address]; !ok {
			addrAssetPair[vout.Address] = make(map[string]bool)
		}
		addrAssetPair[vout.Address][vout.AssetID] = true
	}

	return assetIDs, addrAssetPair
}

func updateTxInfo(tx *sql.Tx, blockTime uint64, txID string, addrs []string, assetIDs map[string]bool, addrAssetPair map[string]map[string]bool) error {
	for _, addr := range addrs {
		// Add new AddrTx record.
		const insertAddrTx = "INSERT INTO `addr_tx` (`txid`, `address`, `block_time`, `asset_type`) VALUES (?, ?, ?, ?)"
		if _, err := tx.Exec(insertAddrTx, txID, addr, blockTime, asset.ASSET); err != nil {
			return err
		}

		for assetID := range addrAssetPair[addr] {
			// Increase transaction count in addr_asset.
			const query = "UPDATE `addr_asset` SET `transactions` = `transactions` + 1, `last_transaction_time` = ? WHERE `address` = ? AND `asset_id` = ? LIMIT 1"
			if _, err := tx.Exec(query, blockTime, addr, assetID); err != nil {
				return err
			}
		}
	}

	for assetID := range assetIDs {
		// Increase asset transactions count.
		const query = "UPDATE `asset` SET `transactions` = `transactions` + 1 WHERE `asset_id` = ? LIMIT 1"
		if _, err := tx.Exec(query, assetID); err != nil {
			return err
		}
	}
	return nil
}

// GetVout returns vouts of a transaction.
func GetVout(txID string, n uint16) (*tx.TransactionVout, error) {
	vout := new(tx.TransactionVout)
	valueStr := ""
	const query = "SELECT `txid`, `n`, `asset_id`, `value`, `address` FROM `tx_vout` WHERE `txid` = ? AND `n` = ?"
	err := db.QueryRow(query, txID, n).Scan(
		// &vout.ID,
		&vout.TxID,
		&vout.N,
		&vout.AssetID,
		&valueStr,
		&vout.Address,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if *vout == (tx.TransactionVout{}) {
		return nil, nil
	}

	vout.Value = util.StrToBigFloat(valueStr)
	return vout, nil
}

// GetHighestTxPk returns maximum pk of tx.
func GetHighestTxPk() uint {
	var pk uint
	const query = "SELECT `id` FROM `tx` WHERE EXISTS (SELECT `id` FROM `tx_vin` WHERE `from`=`tx`.`txid` LIMIT 1) OR EXISTS (SELECT `id` FROM `tx_vout` WHERE `txid`=`tx`.`txid` LIMIT 1) ORDER BY `id` DESC LIMIT 1"
	err := db.QueryRow(query).Scan(&pk)
	if err != nil && err != sql.ErrNoRows {
		if !connErr(err) {
			panic(err)
		}
		reconnect()
		return GetHighestTxPk()
	}

	return pk
}
