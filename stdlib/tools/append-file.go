package tools

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var filesMap map[string]struct{} = make(map[string]struct{})
var filesMapLock sync.RWMutex = sync.RWMutex{}

func AppendFile(fname string, text string) {
	filesMapLock.Lock()
	defer filesMapLock.Unlock()
	_, exists := filesMap[fname]
	if !exists {
		filesMap[fname] = struct{}{}
		// rename current file if it exists
		if _, err := os.Stat(fname); err == nil {
			_ = os.Rename(fname, fmt.Sprintf("%s.%d.old", fname, time.Now().Unix()))
		}
	}
	// append to log file fname, create it if it doesn't exist
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("failed opening file: %s\n", err)
		return
	}

	defer f.Close()

	if _, err := f.WriteString("--=== new report ===--\n" + text + "\n"); err != nil {
		fmt.Printf("failed writing to file: %s\n", err)
		return
	}
}
