package log

import (
	"fmt"
	"os"
	"time"
)

const timeLayout = "2006-01-02 15:04:05"

func stamp() string {
	return time.Now().Format(timeLayout)
}

// Info writes a timestamped INFO line to stdout.
func Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s INFO  %s\n", stamp(), msg)
}

// Error writes a timestamped ERROR line to stdout.
func Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s ERROR %s\n", stamp(), msg)
}
