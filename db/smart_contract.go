package db

import (
	"squirrel/log"
	"squirrel/nep5"
)

// InsertSCInfo persists new smart contract info into db.
func InsertSCInfo(scriptHash string, regInfo *nep5.RegInfo) error {
	const query = "INSERT INTO `smartcontract_info`(`script_hash`, `name`, `version`, `author`, `email`, `description`, `need_storage`, `parameter_list`, `return_type`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
	_, err := db.Exec(query, scriptHash, regInfo.Name, regInfo.Version, regInfo.Author, regInfo.Email, regInfo.Description, regInfo.NeedStorage, regInfo.ParameterList, regInfo.ReturnType)

	if err != nil {
		log.Error.Println(err)
	}

	return err
}
