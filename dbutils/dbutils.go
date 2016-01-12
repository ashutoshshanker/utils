package dbutils

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
)

func ConvertBoolToInt(val bool) int {
	if val {
		return 1
	}
	return 0
}
func ConvertStrBoolIntToBool(val string) bool {
	fmt.Println("ConvertStrBoolIntToBool: val", val)
	if val == "true" {
		return true
	} else if val == "True" {
		return true
	}
	return false
}
func ExecuteSQLStmt(dbCmd string, dbHdl *sql.DB) (driver.Result, error) {
	var result driver.Result
	txn, err := dbHdl.Begin()
	if err != nil {
		fmt.Println("### Failed to strart db transaction for command", dbCmd)
		return result, err
	}
	result, err = dbHdl.Exec(dbCmd)
	if err != nil {
		fmt.Println("### Failed to execute command ", dbCmd, err)
		return result, err
	}
	err = txn.Commit()
	if err != nil {
		fmt.Println("### Failed to Commit transaction for command", dbCmd, err)
		return result, err
	}
	return result, err
}
