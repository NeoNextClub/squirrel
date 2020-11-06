package db

import (
	"database/sql"
	"fmt"
	"math/big"
	"sort"
	"squirrel/asset"
	"squirrel/cache"
	"squirrel/log"
	"squirrel/nft"
	"squirrel/tx"
	"squirrel/util"
	"strings"
)

//GetNftInvocationTxs returns invocation transactions.
func GetNftInvocationTxs(startPk uint, limit uint) []*tx.Transaction {
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

// GetNftAssetDecimals returns all nft asset_id with decimal.
func GetNftAssetDecimals() map[string]uint8 {
	nftDecimals := make(map[string]uint8)
	const query = "SELECT `asset_id`, `decimals` FROM `nft`"
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
		nftDecimals[assetID] = decimal
	}

	return nftDecimals
}

// NftTokenExists tells if the given tokenID exists.
func NftTokenExists(assetID, tokenID string) bool {
	exists := false

	query := "SELECT EXISTS (SELECT `id` FROM `nft_token` WHERE `asset_id` = ? AND `token_id` = ? LIMIT 1)"
	err := db.QueryRow(query, assetID, tokenID).Scan(&exists)
	if err != nil {
		log.Error.Println(err)
		panic(err)
	}

	return exists
}

//GetNftTxScripts returns script string of transaction.
func GetNftTxScripts(txID string) ([]*tx.TransactionScripts, error) {
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

// InsertNftAsset inserts new nft asset into db.
func InsertNftAsset(trans *tx.Transaction, nft *nft.Nft, regInfo *nft.NftRegInfo, atHeight uint) error {
	return transact(func(tx *sql.Tx) error {
		insertNftSQL := fmt.Sprintf("INSERT INTO `nft` (`asset_id`, `admin_address`, `name`, `symbol`, `decimals`, `total_supply`, `txid`, `block_index`, `block_time`, `addresses`, `holding_addresses`, `transfers`) VALUES('%s', '%s', '%s', '%s', %d, %.64f, '%s', %d, %d, %d, %d, %d)", nft.AssetID, nft.AdminAddress, nft.Name, nft.Symbol, nft.Decimals, nft.TotalSupply, nft.TxID, nft.BlockIndex, nft.BlockTime, nft.Addresses, nft.HoldingAddresses, nft.Transfers)
		res, err := tx.Exec(insertNftSQL)
		if err != nil {
			return err
		}

		newPK, err := res.LastInsertId()
		if err != nil {
			return err
		}
		const insertNftRegInfo = "INSERT INTO `nft_reg_info` (`nft_id`, `name`, `version`, `author`, `email`, `description`, `need_storage`, `parameter_list`, `return_type`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
		if _, err := tx.Exec(insertNftRegInfo, newPK, regInfo.Name, regInfo.Version, regInfo.Author, regInfo.Email, regInfo.Description, regInfo.NeedStorage, regInfo.ParameterList, regInfo.ReturnType); err != nil {
			return err
		}

		return updateNftCounter(tx, trans.ID, -1)
	})
}

// UpdateNftTotalSupplyAndAddrAsset updates nft total supply.
func UpdateNftTotalSupplyAndAddrAsset(blockTime uint64, blockIndex uint, assetID string, totalSupply *big.Float) error {
	return transact(func(tx *sql.Tx) error {
		return UpdateNftTotalSupply(tx, assetID, totalSupply)
	})
}

// UpdateNftTotalSupply updates total supply of nft asset.
func UpdateNftTotalSupply(tx *sql.Tx, assetID string, totalSupply *big.Float) error {
	query := fmt.Sprintf("UPDATE `nft` SET `total_supply` = %.64f WHERE `asset_id` = '%s' LIMIT 1", totalSupply, assetID)

	_, err := tx.Exec(query)

	return err
}

// InsertNftTransaction inserts new nft transaction into db.
func InsertNftTransaction(trans *tx.Transaction, appLogIdx int, assetID string, fromAddr string, fromBalance *big.Float, toAddr string, toBalance *big.Float, transferValue *big.Float, tokenID string, totalSupply *big.Float, nftJSONInfo string) error {
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

			addrCreated, err := updateAddrInfo(tx, trans.BlockTime, trans.TxID, addr, asset.NFT)
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

			var recordExists bool
			query := "SELECT EXISTS(SELECT `id` FROM `addr_asset_nft` WHERE `address`=? AND `asset_id`=? AND `token_id`=?)"
			err = tx.QueryRow(query, addr, assetID, tokenID).Scan(&recordExists)
			if err != nil {
				return err
			}

			// Insert addr_asset_nft record if not exist or update record.
			if !recordExists {
				insertAddrAssetQuery := fmt.Sprintf("INSERT INTO `addr_asset_nft` (`address`, `asset_id`, `token_id`, `balance`) VALUES ('%s', '%s', '%s', %.18f)", addr, assetID, tokenID, transferValue)
				if _, err := tx.Exec(insertAddrAssetQuery); err != nil {
					return err
				}
			} else {
				addrAssetCache.UpdateBalance(balance, trans.BlockIndex)
				op := "+"
				if addr == fromAddr {
					op = "-"
				}

				updateAddrAssetQuery := fmt.Sprintf("UPDATE `addr_asset_nft` SET `balance` = `balance` %s %.64f WHERE `address` = ? AND `asset_id` = ? AND `token_id`= ? LIMIT 1", op, transferValue)
				if _, err := tx.Exec(updateAddrAssetQuery, addr, assetID, tokenID); err != nil {
					return err
				}
			}
		}

		// Update nft transactions and addresses counter.
		txSQL := fmt.Sprintf("UPDATE `nft` SET `addresses` = `addresses` + %d, `holding_addresses` = `holding_addresses` + %d, `transfers` = `transfers` + 1 WHERE `asset_id` = '%s' LIMIT 1;", addrsOffset, holdingAddrsOffset, assetID)

		// Insert nft transaction record.
		txSQL += fmt.Sprintf("INSERT INTO `nft_tx` (`txid`, `asset_id`, `from`, `to`, `token_id`, `value`, `block_index`, `block_time`) VALUES ('%s', '%s', '%s', '%s', '%s', %.64f, %d, %d);", trans.TxID, assetID, fromAddr, toAddr, tokenID, transferValue, trans.BlockIndex, trans.BlockTime)

		// Handle resultant of storage injection attach.
		if totalSupply != nil {
			txSQL += fmt.Sprintf("UPDATE `nft` SET `total_supply` = %.64f WHERE `asset_id` = '%s' LIMIT 1;", totalSupply, assetID)
		}

		if _, err := tx.Exec(txSQL); err != nil {
			return err
		}

		if addrCreatedCnt > 0 {
			if err := incrAddrCounter(tx, addrCreatedCnt); err != nil {
				return err
			}
		}

		if nftJSONInfo != "" {
			if err := persistNftToken(tx, assetID, tokenID, nftJSONInfo); err != nil {
				return err
			}
		}

		err := updateNftCounter(tx, trans.ID, appLogIdx)
		return err
	})
}

func persistNftToken(tx *sql.Tx, assetID, tokenID, nftJSONInfo string) error {
	query := "INSERT INTO `nft_token`(`asset_id`, `token_id`, `info`) VALUES(?, ?, ?)"
	_, err := tx.Exec(query, assetID, tokenID, nftJSONInfo)
	if err != nil {
		log.Error.Println(err)
	}

	return err
}

// GetMaxNonEmptyScriptTxPkForNft returns largest pk of invocation transaction.
func GetMaxNonEmptyScriptTxPkForNft() uint {
	const query = "SELECT `id` from `tx` WHERE `type` = ? ORDER BY `id` DESC LIMIT 1"

	var pk uint
	err := db.QueryRow(query, "InvocationTransaction").Scan(&pk)
	if err != nil && err != sql.ErrNoRows {
		if !connErr(err) {
			panic(err)
		}
		reconnect()
		return GetMaxNonEmptyScriptTxPkForNft()
	}

	return pk
}

// GetNftTxRecords returns paged nft transactions from db.
func GetNftTxRecords(pk uint, limit int) ([]*nft.Transaction, error) {
	const query = "SELECT `id`, `txid`, `asset_id`, `from`, `to`, `value`, `block_index`, `block_time` FROM `nft_tx` WHERE `id` > ? ORDER BY `id` ASC LIMIT ?"
	rows, err := wrappedQuery(query, pk, limit)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	records := []*nft.Transaction{}

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

		record := &nft.Transaction{
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

// InsertNftAddrTxRec inserts addr_tx record of nft transactions.
func InsertNftAddrTxRec(nftTxRecs []*nft.Transaction, lastPk uint) error {
	if len(nftTxRecs) == 0 {
		return nil
	}

	return transact(func(tx *sql.Tx) error {
		var strBuilder strings.Builder

		strBuilder.WriteString("INSERT INTO `addr_tx` (`txid`, `address`, `block_time`, `asset_type`) VALUES ")

		for _, rec := range nftTxRecs {
			if len(rec.From) > 0 {
				strBuilder.WriteString(fmt.Sprintf("('%s', '%s', %d, '%s'),", rec.TxID, rec.From, rec.BlockTime, asset.NFT))
			}
			if len(rec.To) > 0 {
				strBuilder.WriteString(fmt.Sprintf("('%s', '%s', %d, '%s'),", rec.TxID, rec.To, rec.BlockTime, asset.NFT))
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

		return UpdateNftTxPkForAddrTx(tx, lastPk)
	})
}
