package main

import (
	"flag"
	_ "net/http/pprof"
	"squirrel/config"
	"squirrel/db"
	"squirrel/log"
	"squirrel/mail"
	"squirrel/rpc"
	"squirrel/tasks"
)

var enableMail bool

func init() {
	flag.BoolVar(&enableMail, "mail", false, "If mail alert is enabled")
}

func main() {
	flag.Parse()

	log.Init()
	config.Load(true)
	db.Init()
	mail.Init(enableMail)

	defer mail.AlertIfErr()

	go rpc.TraceBestHeight()

	tasks.Run()

	select {}
}
