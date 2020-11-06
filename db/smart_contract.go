package db

import (
	"database/sql"
	"squirrel/nep5"
	"squirrel/util"
)

// InsertSCInfos persists new smart contracts info into db.
func InsertSCInfos(scRegInfos []*nep5.RegInfo, txPK uint) error {
	if len(scRegInfos) == 0 {
		return UpdateLastTxPkForSC(txPK)
	}

	return transact(func(trans *sql.Tx) error {
		query := "INSERT INTO `smartcontract_info`(`txid`, `script_hash`, `name`, `version`, `author`, `email`, `description`, `need_storage`, `parameter_list`, `return_type`) VALUES "
		args := []interface{}{}

		for _, regInfo := range scRegInfos {
			scriptHashHex := util.GetAssetIDFromScriptHash(regInfo.ScriptHash)
			query += "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?), "
			args = append(args, regInfo.TxID, scriptHashHex, regInfo.Name, regInfo.Version, regInfo.Author, regInfo.Email, regInfo.Description, regInfo.NeedStorage, regInfo.ParameterList, regInfo.ReturnType)
		}

		_, err := trans.Exec(query[:len(query)-2], args...)
		if err != nil {
			panic(err)
		}

		return updateCounter(trans, "last_tx_pk_for_sc", int64(txPK))
	})
}
