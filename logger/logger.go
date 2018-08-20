package logger

import (
	"github.com/op/go-logging"
	"os"
)

// Example format string. Everything except the message has a custom color
// which is dependent on the logger level. Many fields have a custom output
// formatting too, eg. the time returns the hour down to the milli second.
//%{id}        Sequence number for logger message (uint64).
//%{pid}       Process id (int)
//%{time}      Time when logger occurred (time.Time)
//%{level}     Log level (Level)
//%{module}    Module (string)
//%{program}   Basename of os.Args[0] (string)
//%{message}   Message (string)
//%{longfile}  Full file name and line number: /a/b/c/d.go:23
//%{shortfile} Final file name element and line number: d.go:23
//%{callpath}  Callpath like main.a.b.c...c  "..." meaning recursive call ~. meaning truncated path
//%{color}     ANSI color based on logger level
var format = logging.MustStringFormatter(
	`%{color}%{time:15:04:05.000} %{level:.5s} %{pid} %{module} %{shortfile} %{color:reset} %{message}`,
)

func init() {
	// For demo purposes, create two backend for os.Stderr.
	backend := logging.NewLogBackend(os.Stdout, "", 0)

	// For messages written to backend2 we want to add some additional
	// information to the output, including the used logger level and the name of
	// the function.
	backendFormatter := logging.NewBackendFormatter(backend, format)

	// Only errors and more severe messages should be sent to backend1
	backendLeveled := logging.AddModuleLevel(backend)
	backendLeveled.SetLevel(logging.ERROR, "")

	// Set the backends to be used.
	logging.SetBackend(backendLeveled, backendFormatter)

	//logging.SetLevel(logging.INFO, "")
}

func NewLogger(module string) *logging.Logger {
	logger := logging.MustGetLogger(module)

	return logger
}
