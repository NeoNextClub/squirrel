package db

import (
	"database/sql"
	"fmt"
	"math/big"
	"squirrel/tx"
	"squirrel/util"
	"strings"
)

// GasDateBalance is the struct to store GAS balance-date values.
type GasDateBalance struct {
	Date    string
	Balance *big.Float
}

var gasDateCache = make(map[string]*GasDateBalance)

// ApplyGASAssetChange persists daily gas balance changes into DB.
func ApplyGASAssetChange(tx *tx.Transaction, date string, gasChangeMap map[string]*big.Float) error {
	for addr, gasChange := range gasChangeMap {
		err := transact(func(trans *sql.Tx) error {
			gasDateBalanceCache, ok := gasDateCache[addr]
			if !ok {
				dataCache := GasDateBalance{
					Date:    date,
					Balance: gasChange,
				}

				gasDateCache[addr] = &dataCache

				lastDate, balance := queryAddrGasDateRecord(addr)
				if balance == nil || lastDate != date {
					if balance != nil {
						dataCache.Balance = new(big.Float).SetPrec(256).Add(balance, gasChange)
					}

					err := insertGasDateBalanceRecord(trans, addr, date, dataCache.Balance)
					if err != nil {
						return err
					}
				} else {
					dataCache.Balance = new(big.Float).SetPrec(256).Add(balance, gasChange)
					err := updateGasDateBalanceRecord(trans, addr, date, dataCache.Balance)
					if err != nil {
						return err
					}
				}

				err := updateCounter(trans, "last_tx_pk_gas_balance", int64(tx.ID))
				return err
			}

			newBalance := new(big.Float).SetPrec(256).Add(gasDateBalanceCache.Balance, gasChange)
			gasDateBalanceCache.Balance = newBalance

			if gasDateBalanceCache.Date == date {
				updateGasDateBalanceRecord(trans, addr, date, newBalance)
			} else {
				gasDateBalanceCache.Date = date
				insertGasDateBalanceRecord(trans, addr, date, newBalance)
			}

			err := updateCounter(trans, "last_tx_pk_gas_balance", int64(tx.ID))
			return err
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func queryAddrGasDateRecord(addr string) (string, *big.Float) {
	tableName := getAddrDateGasTableName(addr)
	query := fmt.Sprintf("SELECT `date`, `balance` FROM `%s` ", tableName)
	query += fmt.Sprintf("WHERE `address` = '%s' ", addr)
	query += "ORDER BY `id` DESC LIMIT 1"

	var date string
	var balanceStr string
	err := db.QueryRow(query).Scan(&date, &balanceStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}

		if !connErr(err) {
			panic(err)
		}

		reconnect()
		return queryAddrGasDateRecord(addr)
	}

	return date, util.StrToBigFloat(balanceStr)
}

func insertGasDateBalanceRecord(trans *sql.Tx, addr, date string, balance *big.Float) error {
	tableName := getAddrDateGasTableName(addr)
	query := fmt.Sprintf("INSERT INTO `%s`(`address`, `date`, `balance`) ", tableName)
	query += fmt.Sprintf("VALUES ('%s', '%s', %.8f)", addr, date, balance)

	_, err := trans.Exec(query)
	return err
}

func updateGasDateBalanceRecord(trans *sql.Tx, addr, date string, gasChange *big.Float) error {
	tableName := getAddrDateGasTableName(addr)
	query := fmt.Sprintf("UPDATE `%s` ", tableName)
	query += fmt.Sprintf("SET `balance` = %.8f ", gasChange)
	query += fmt.Sprintf("WHERE `address` = '%s' and `date` = '%s' ", addr, date)
	query += "LIMIT 1"

	_, err := trans.Exec(query)
	if err != nil {
		if !connErr(err) {
			panic(err)
		}

		reconnect()
		return updateGasDateBalanceRecord(trans, addr, date, gasChange)
	}

	return nil
}

func getAddrDateGasTableName(addr string) string {
	suffix := strings.ToLower(string(addr[len(addr)-1]))
	return "addr_gas_balance_" + suffix
}
