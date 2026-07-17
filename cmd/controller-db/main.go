package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"neon-selfhost/internal/controllerdb"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: controller-db {backup|restore|validate-archive}")
	}

	action := os.Args[1]
	if action == "validate-archive" {
		if len(os.Args) != 3 {
			fail("usage: controller-db validate-archive /path/to/controller-state.tar")
		}
		if err := controllerdb.ValidateVolumeArchive(os.Args[2]); err != nil {
			fail("validate controller volume archive: %v", err)
		}
		return
	}
	if len(os.Args) != 2 {
		fail("usage: controller-db {backup|restore|validate-archive}")
	}

	dataDir := strings.TrimSpace(os.Getenv("CONTROLLER_DATA_DIR"))
	if dataDir == "" {
		fail("set CONTROLLER_DATA_DIR=/path/to/controller-data")
	}
	backupDir := strings.TrimSpace(os.Getenv("BACKUP_DIR"))

	switch action {
	case "backup":
		if backupDir == "" {
			backupDir = filepath.Join(dataDir, "backup-"+time.Now().UTC().Format("20060102-150405"))
		}
		if err := controllerdb.Backup(dataDir, backupDir); err != nil {
			fail("backup controller databases: %v", err)
		}
		fmt.Printf("backup created at %s\n", backupDir)
	case "restore":
		if backupDir == "" {
			fail("set BACKUP_DIR=/path/to/backup-dir")
		}
		if err := controllerdb.Restore(dataDir, backupDir); err != nil {
			fail("restore controller databases: %v", err)
		}
		fmt.Printf("restore completed from %s\n", backupDir)
	default:
		fail("usage: controller-db {backup|restore|validate-archive}")
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
