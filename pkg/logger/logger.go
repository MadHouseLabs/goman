package logger

import (
	"log"
	"os"
)

var silent bool

func init() {
	// Silent in TUI mode (default), verbose in Lambda or debug mode
	silent = os.Getenv("AWS_LAMBDA_RUNTIME_API") == "" && os.Getenv("GOMAN_DEBUG") != "true"
}

// SetSilent allows runtime control of logging
func SetSilent(s bool) {
	silent = s
}

// Printf logs formatted output if not in silent mode
func Printf(format string, v ...interface{}) {
	if !silent {
		log.Printf(format, v...)
	}
}

// Println logs output if not in silent mode
func Println(v ...interface{}) {
	if !silent {
		log.Println(v...)
	}
}