package db

import (
	"database/sql"
	"fmt"
	"math/big"
	"sort"
	"squirrel/addr"
	"squirrel/asset"
	"squirrel/cache"
	"squirrel/log"
	"squirrel/nep5"
	"squirrel/tx"
	"squirrel/util"
	"strings"
)

type addrInfo struct {
	addr    string
	balance *big.Float
}

// GetInvocationTxs returns invocation transactions.
func GetInvocationTxs(startPk uint, limit uint) []*tx.Transaction {
	const query = "SELECT `id`, `block_index`, `block_time`, `txid`, `size`, `type`, `version`, `sys_fee`, `net_fee`, `nonce`, `script`, `gas` FROM `tx` WHERE `id` >= ? AND `type` = ? ORDER BY ID ASC LIMIT ?"
	rows, err := wrappedQuery(query, startPk, "InvocationTransaction", limit)
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

// GetNep5AssetDecimals returns all nep5 asset_id with decimal.
func GetNep5AssetDecimals() map[string]uint8 {
	nep5Decimals := make(map[string]uint8)
	const query = "SELECT `asset_id`, `decimals` FROM `nep5`"
	rows, err := wrappedQuery(query)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var assetID string
		var decimal uint8
		if err := rows.Scan(&assetID, &decimal); err != nil {
			panic(err)
		}
		nep5Decimals[assetID] = decimal
	}

	return nep5Decimals
}

// GetTxScripts returns script string of transaction.
func GetTxScripts(txID string) ([]*tx.TransactionScripts, error) {
	var txScripts []*tx.TransactionScripts
	const query = "SELECT `id`, `txid`, `invocation`, `verification` FROM `tx_scripts` WHERE `txid` = ?"
	rows, err := wrappedQuery(query, txID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		txScript := tx.TransactionScripts{}
		rows.Scan(
			&txScript.ID,
			&txScript.TxID,
			&txScript.Invocation,
			&txScript.Verification,
		)
		txScripts = append(txScripts, &txScript)
	}

	return txScripts, nil
}

// InsertNep5Asset inserts new nep5 asset into db.
func InsertNep5Asset(trans *tx.Transaction, nep5 *nep5.Nep5, regInfo *nep5.RegInfo, addrAsset *addr.Asset, atHeight uint) error {
	return transact(func(tx *sql.Tx) error {
		insertNep5Sql := fmt.Sprintf("INSERT INTO `nep5` (`asset_id`, `admin_address`, `name`, `symbol`, `decimals`, `total_supply`, `txid`, `block_index`, `block_time`, `addresses`, `holding_addresses`, `transfers`) VALUES('%s', '%s', '%s', '%s', %d, %.64f, '%s', %d, %d, %d, %d, %d)", nep5.AssetID, nep5.AdminAddress, nep5.Name, nep5.Symbol, nep5.Decimals, nep5.TotalSupply, nep5.TxID, nep5.BlockIndex, nep5.BlockTime, nep5.Addresses, nep5.HoldingAddresses, nep5.Transfers)
		res, err := tx.Exec(insertNep5Sql)
		if err != nil {
			return err
		}

		newPK, err := res.LastInsertId()
		if err != nil {
			return err
		}
		const insertNep5RegInfo = "INSERT INTO `nep5_reg_info` (`nep5_id`, `name`, `version`, `author`, `email`, `description`, `need_storage`, `parameter_list`, `return_type`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
		if _, err := tx.Exec(insertNep5RegInfo, newPK, regInfo.Name, regInfo.Version, regInfo.Author, regInfo.Email, regInfo.Description, regInfo.NeedStorage, regInfo.ParameterList, regInfo.ReturnType); err != nil {
			return err
		}

		addrCreated := false
		if addrAsset != nil {
			addrCreated, err = createAddrInfoIfNotExist(tx, trans.BlockTime, addrAsset.Address)
			if err != nil {
				log.Error.Printf("TxID: %s, nep5Info: %+v, regInfo=%+v, addrAsset=%+v, atHeight=%d\n", trans.TxID, nep5, regInfo, addrAsset, atHeight)
				return err
			}

			if _, ok := cache.GetAddrAsset(addrAsset.Address, addrAsset.AssetID); !ok {
				cache.CreateAddrAsset(addrAsset.Address, addrAsset.AssetID, addrAsset.Balance, atHeight)
				insertAddrAssetQuery := fmt.Sprintf("INSERT INTO `addr_asset` (`address`, `asset_id`, `balance`, `transactions`, `last_transaction_time`) VALUES ('%s', '%s', %.64f, %d, %d)", addrAsset.Address, addrAsset.AssetID, addrAsset.Balance, addrAsset.Transactions, addrAsset.LastTransactionTime)
				if _, err := tx.Exec(insertAddrAssetQuery); err != nil {
					return err
				}
			}
		}

		if addrCreated {
			if err := incrAddrCounter(tx, 1); err != nil {
				return err
			}
		}
		return updateNep5Counter(tx, trans.ID, -1)
	})
}

// UpdateNep5TotalSupplyAndAddrAsset updates nep5 total supply and admin balance.
func UpdateNep5TotalSupplyAndAddrAsset(blockTime uint64, blockIndex uint, addr string, balance *big.Float, assetID string, totalSupply *big.Float) error {
	return transact(func(tx *sql.Tx) error {
		addrCreated := false
		var err error

		if balance.Cmp(big.NewFloat(0)) == 1 {
			if addrCreated, err = createAddrInfoIfNotExist(tx, blockTime, addr); err != nil {
				log.Error.Printf("blockTime=%d, blockIndex=%d, addr=%s, balance=%v, assetID=%s, totalSupply=%v\n",
					blockTime, blockIndex, addr, balance, assetID, totalSupply)
				return err
			}

			cachedAddr, _ := cache.GetAddrOrCreate(addr, blockTime)
			addrAssetCache, created := cachedAddr.GetAddrAssetOrCreate(assetID, balance)

			if created {
				insertAddrAssetQuery := fmt.Sprintf("INSERT INTO `addr_asset` (`address`, `asset_id`, `balance`, `transactions`, `last_transaction_time`) VALUES ('%s', '%s', %.64f, %d, %d)", addr, assetID, balance, 0, blockTime)
				if _, err := tx.Exec(insertAddrAssetQuery); err != nil {
					return err
				}
				const incrNep5AddrQuery = "UPDATE `nep5` SET `addresses` = `addresses` + 1, `holding_addresses` = `holding_addresses` + 1 WHERE `asset_id` = ? LIMIT 1"
				if _, err := tx.Exec(incrNep5AddrQuery, assetID); err != nil {
					return err
				}
			} else {
				if addrAssetCache.UpdateBalance(balance, blockIndex) {
					query := fmt.Sprintf("UPDATE `addr_asset` SET `balance` = %.64f WHERE `address` = '%s' AND `asset_id` = '%s' LIMIT 1", balance, addr, assetID)
					if _, err := tx.Exec(query); err != nil {
						return err
					}
				}
			}
		} else {
			// balance is zero.
			if addrAssetCache, ok := cache.GetAddrAsset(addr, assetID); ok {
				if addrAssetCache.UpdateBalance(balance, blockIndex) {
					const updateBalanceQuery = "UPDATE `nep5` SET `holding_addresses` = `holding_addresses` - 1 WHERE `asset_id` = ? LIMIT 1"
					if _, err := tx.Exec(updateBalanceQuery, assetID); err != nil {
						return err
					}
				}
			}
		}

		// Update nep5 total supply.
		if err := UpdateNep5TotalSupply(tx, assetID, totalSupply); err != nil {
			return err
		}

		if addrCreated {
			return incrAddrCounter(tx, 1)
		}

		return nil
	})
}

// UpdateNep5TotalSupply updates total supply of nep5 asset.
func UpdateNep5TotalSupply(tx *sql.Tx, assetID string, totalSupply *big.Float) error {
	query := fmt.Sprintf("UPDATE `nep5` SET `total_supply` = %.64f WHERE `asset_id` = '%s' LIMIT 1", totalSupply, assetID)

	_, err := tx.Exec(query)

	return err
}

// InsertNep5transaction inserts new nep5 transaction into db.
func InsertNep5transaction(trans *tx.Transaction, appLogIdx int, assetID string, fromAddr string, fromBalance *big.Float, toAddr string, toBalance *big.Float, transferValue *big.Float, totalSupply *big.Float) error {
	return transact(func(tx *sql.Tx) error {
		addrsOffset := 0
		holdingAddrsOffset := 0

		addrInfoPair := []addrInfo{
			{addr: fromAddr, balance: fromBalance},
			{addr: toAddr, balance: toBalance},
		}

		// Handle special case.
		if fromAddr == toAddr {
			addrInfoPair = addrInfoPair[:1]
		} else {
			// Sort address to avoid potential deadlock.
			sort.SliceStable(addrInfoPair, func(i, j int) bool {
				return addrInfoPair[i].addr < addrInfoPair[j].addr
			})
		}

		addrCreatedCnt := 0

		for _, info := range addrInfoPair {
			addr := info.addr
			balance := info.balance

			if len(addr) == 0 {
				continue
			}

			addrCreated, err := updateAddrInfo(tx, trans.BlockTime, trans.TxID, addr, asset.NEP5)
			if err != nil {
				return err
			}
			if addrCreated {
				addrCreatedCnt++
			}

			cachedAddr, _ := cache.GetAddrOrCreate(addr, trans.BlockTime)
			addrAssetCache, created := cachedAddr.GetAddrAssetOrCreate(assetID, balance)

			if balance.Cmp(big.NewFloat(0)) == 1 {
				if created || addrAssetCache.Balance.Cmp(big.NewFloat(0)) == 0 {
					holdingAddrsOffset++
				}
			} else { // have no balance currently.
				if !created && addrAssetCache.Balance.Cmp(big.NewFloat(0)) == 1 {
					holdingAddrsOffset--
				}
			}

			if created {
				addrsOffset++
			}

			// Insert addr_asset record if not exist or update record.
			if created {
				insertAddrAssetQuery := fmt.Sprintf("INSERT INTO `addr_asset` (`address`, `asset_id`, `balance`, `transactions`, `last_transaction_time`) VALUES ('%s', '%s', %.64f, %d, %d)", addr, assetID, balance, 1, trans.BlockTime)
				if _, err := tx.Exec(insertAddrAssetQuery); err != nil {
					return err
				}
			} else {
				addrAssetCache.UpdateBalance(balance, trans.BlockIndex)
				updateAddrAssetQuery := fmt.Sprintf("UPDATE `addr_asset` SET `balance` = %.64f, `transactions` = `transactions` + 1, `last_transaction_time` = %d WHERE `address` = '%s' AND `asset_id` = '%s' LIMIT 1", balance, trans.BlockTime, addr, assetID)
				if _, err := tx.Exec(updateAddrAssetQuery); err != nil {
					return err
				}
			}
		}

		// Update nep5 transactions and addresses counter.
		txSQL := fmt.Sprintf("UPDATE `nep5` SET `addresses` = `addresses` + %d, `transfers` = `transfers` + 1 WHERE `asset_id` = '%s' LIMIT 1;", addrsOffset, assetID)
		if holdingAddrsOffset != 0 {
			txSQL += fmt.Sprintf("UPDATE `nep5` SET `holding_addresses` = `holding_addresses` + %d WHERE `asset_id` = '%s' AND `holding_addresses` >= %d LIMIT 1;", holdingAddrsOffset, assetID, -holdingAddrsOffset)
		}

		// Insert nep5 transaction record.
		txSQL += fmt.Sprintf("INSERT INTO `nep5_tx` (`txid`, `asset_id`, `from`, `to`, `value`, `block_index`, `block_time`) VALUES ('%s', '%s', '%s', '%s', %.64f, %d, %d);", trans.TxID, assetID, fromAddr, toAddr, transferValue, trans.BlockIndex, trans.BlockTime)

		// Handle resultant of storage injection attach.
		if totalSupply != nil {
			txSQL += fmt.Sprintf("UPDATE `nep5` SET `total_supply` = %.64f WHERE `asset_id` = '%s' LIMIT 1;", totalSupply, assetID)
		}

		if _, err := tx.Exec(txSQL); err != nil {
			return err
		}

		if addrCreatedCnt > 0 {
			if err := incrAddrCounter(tx, addrCreatedCnt); err != nil {
				return err
			}
		}
		err := updateNep5Counter(tx, trans.ID, appLogIdx)
		return err
	})
}

// GetMaxNonEmptyScriptTxPk returns largest pk of invocation transaction.
func GetMaxNonEmptyScriptTxPk() uint {
	const query = "SELECT `id` from `tx` WHERE `type` = ? ORDER BY `id` DESC LIMIT 1"

	var pk uint
	err := db.QueryRow(query, "InvocationTransaction").Scan(&pk)
	if err != nil && err != sql.ErrNoRows {
		if !connErr(err) {
			panic(err)
		}
		reconnect()
		return GetMaxNonEmptyScriptTxPk()
	}

	return pk
}

// GetNep5TxRecords returns paged nep5 transactions from db.
func GetNep5TxRecords(pk uint, limit int) ([]*nep5.Transaction, error) {
	const query = "SELECT `id`, `txid`, `asset_id`, `from`, `to`, `value`, `block_index`, `block_time` FROM `nep5_tx` WHERE `id` > ? ORDER BY `id` ASC LIMIT ?"
	rows, err := wrappedQuery(query, pk, limit)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	records := []*nep5.Transaction{}

	for rows.Next() {
		var id uint
		var txID string
		var assetID string
		var from string
		var to string
		var valueStr string
		var blockIndex uint
		var blockTime uint64

		err := rows.Scan(&id, &txID, &assetID, &from, &to, &valueStr, &blockIndex, &blockTime)
		if err != nil {
			return nil, err
		}

		record := &nep5.Transaction{
			ID:         id,
			TxID:       txID,
			AssetID:    assetID,
			From:       from,
			To:         to,
			Value:      util.StrToBigFloat(valueStr),
			BlockIndex: blockIndex,
			BlockTime:  blockTime,
		}
		records = append(records, record)
	}

	return records, nil
}

// InsertNep5AddrTxRec inserts addr_tx record of nep5 transactions.
func InsertNep5AddrTxRec(nep5TxRecs []*nep5.Transaction, lastPk uint) error {
	if len(nep5TxRecs) == 0 {
		return nil
	}

	return transact(func(tx *sql.Tx) error {
		var strBuilder strings.Builder

		strBuilder.WriteString("INSERT INTO `addr_tx` (`txid`, `address`, `block_time`, `asset_type`) VALUES ")

		for _, rec := range nep5TxRecs {
			if len(rec.From) > 0 {
				strBuilder.WriteString(fmt.Sprintf("('%s', '%s', %d, '%s'),", rec.TxID, rec.From, rec.BlockTime, asset.NEP5))
			}
			if len(rec.To) > 0 {
				strBuilder.WriteString(fmt.Sprintf("('%s', '%s', %d, '%s'),", rec.TxID, rec.To, rec.BlockTime, asset.NEP5))
			}
		}
		var query = strBuilder.String()
		if query[len(query)-1] != ',' {
			return nil
		}

		query = strings.TrimSuffix(query, ",")
		query += "ON DUPLICATE KEY UPDATE `address`=`address`"

		if _, err := tx.Exec(query); err != nil {
			return err
		}

		return UpdateNep5TxPkForAddrTx(tx, lastPk)
	})
}
