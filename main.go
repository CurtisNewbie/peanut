package main

import (
	"log"
	"os"

	"github.com/curtisnewbie/gocommon/common"
	"github.com/curtisnewbie/peanut/console"
)

func main() {
	log.SetFlags(0) // remove timestamp in log

	common.DefaultReadConfig(os.Args)
	if e := console.LaunchConsole(); e != nil {
		log.Fatalf("Console encounted fatal error, %v", e)
	}
}
