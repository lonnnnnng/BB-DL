package bbdown

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var archiveMu sync.Mutex

func SaveAidToArchive(aid string) error {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	file, err := os.OpenFile(archivePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(aid + "|")
	return err
}

func CheckAidFromArchive(aid string) bool {
	archiveMu.Lock()
	defer archiveMu.Unlock()
	body, err := os.ReadFile(archivePath())
	if err != nil {
		return false
	}
	for _, item := range strings.Split(string(body), "|") {
		if item == aid {
			return true
		}
	}
	return false
}

func archivePath() string {
	return filepath.Join(AppDir(), "BBDown.archives")
}
