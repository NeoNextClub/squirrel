package db

import (
	"database/sql"
	"squirrel/config"
	"squirrel/log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
)

var (
	db     *sql.DB
	locker uint32
)

// Init connects to the configured mysql database.
func Init() {
	var err error

	db, err = sql.Open("mysql", config.GetDbConnStr())
	if err != nil {
		panic(err)
	}
}

func reconnect() {
	if !atomic.CompareAndSwapUint32(&locker, 0, 1) {
		for {
			// Lock was held by others, wait till lock released.
			time.Sleep(20 * time.Millisecond)
			// Lock was released.
			if atomic.LoadUint32(&locker) != 1 {
				return
			}
		}
	}

	defer atomic.StoreUint32(&locker, 0)

	for {
		log.Printf("Try Reconnecting to database...")
		db, _ = sql.Open("mysql", config.GetDbConnStr())

		if err := db.Ping(); err == nil {
			return
		}

		log.Printf("Wait for few seconds to reconnect again")
		time.Sleep(5 * time.Second)
	}
}

func wrappedQuery(query string, args ...interface{}) (*sql.Rows, error) {
	for {
		rows, err := db.Query(query, args...)
		if err == nil {
			return rows, err
		}

		if !connErr(err) {
			rows.Close()
			return nil, err
		}

		reconnect()
	}
}

func transact(txFunc func(*sql.Tx) error) (err error) {
	tx, err := db.Begin()
	if err != nil {
		if !connErr(err) {
			return err
		}

		reconnect()
		return transact(txFunc)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
			if err != nil {
				log.Error.Println(err)
			}
		}
	}()

	err = txFunc(tx)
	if err == nil || !connErr(err) {
		return err
	}

	reconnect()
	return transact(txFunc)
}

func connErr(err error) bool {
	if err == nil {
		return false
	}

	log.Println(err)

	if err == mysql.ErrInvalidConn ||
		strings.HasSuffix(err.Error(), "operation timed out") ||
		strings.HasSuffix(err.Error(), "Server shutdown in progress") ||
		strings.HasPrefix(err.Error(), "Error 1290") {
		return true
	}

	return false
}
