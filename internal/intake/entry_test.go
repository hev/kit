package intake

import (
	"io"
	"os"
)

func runWatchTest(args []string, out io.Writer) error {
	entry := os.Getenv("HEV_WATCH_TEST_ENTRY")
	if entry == "" {
		entry = "watch"
	}
	return Run(append([]string{entry}, args...), out)
}
