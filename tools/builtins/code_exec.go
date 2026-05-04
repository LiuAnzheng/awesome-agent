package builtins

import (
	"log"
	"os"
)

var logger = log.New(os.Stderr, "[tools/builtins] ", log.LstdFlags|log.Lshortfile)
