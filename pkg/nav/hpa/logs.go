package hpa

import "log"

func debugf(format string, args ...any) {
	if !DebugLogs {
		return
	}
	log.Printf(format, args...)
}
