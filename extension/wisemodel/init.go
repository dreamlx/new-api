// Package wisemodel implements the WiseModel channel-partner extension.
//
// This package is consumed exclusively by the wisemodel-main customer branch
// and must not be imported by any code on the main branch. It bundles all
// WiseModel-specific wiring into a single Init() function so the surface
// area in upstream-tracked files (main.go, model/main.go, ...) stays small.
//
// See docs/UPSTREAM_SYNC_PLAYBOOK.md and the Phase A refactor commit message
// for rationale.
package wisemodel

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var initOnce sync.Once

// Init wires the WiseModel extension into the host process. Call exactly
// once during startup, AFTER model.InitDB() returns and BEFORE the HTTP
// server starts accepting traffic. Safe to call multiple times — only the
// first call has any effect (cron goroutine is launched once).
//
// Effects:
//   - Registers &model.WisemodelPackage{} for AutoMigrate. Note: callers
//     must register BEFORE model.InitDB runs migrateDB(), so the cleanest
//     wiring order in main.go is:
//       wisemodel.RegisterModels()        // before InitDB
//       err := model.InitDB()
//       ...
//       wisemodel.Init()                  // after InitDB; starts cron + migrations
//   - Runs wisemodel-specific BIGINT column type migrations
//     (wisemodel_packages.original_points / original_tokens). The
//     users.quota migration is generic and stays on main as
//     migrateUserQuotaToBigint.
//   - Starts a goroutine that scans for expired packages every 5 minutes
//     and reclaims their unused quota.
func Init() {
	initOnce.Do(func() {
		// 1. Run wisemodel-specific column type migrations.
		if err := migrateWisemodelBigint(); err != nil {
			common.SysError("wisemodel migration failed: " + err.Error())
		}

		// 2. Launch the expired package quota reclaim loop.
		go reclaimExpiredPackagesLoop()
	})
}

// RegisterModels appends WiseModel GORM types to model.extraAutoMigrate.
// Call this from main.go BEFORE model.InitDB() so the migrator picks
// them up on the first run.
func RegisterModels() {
	model.RegisterExtraAutoMigrate(&model.WisemodelPackage{})
}

func reclaimExpiredPackagesLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := model.ReclaimAllExpiredPackages(); err != nil {
			common.SysError("ReclaimAllExpiredPackages failed: " + err.Error())
		}
	}
}

// migrateWisemodelBigint upgrades wisemodel_packages.original_points and
// .original_tokens from INT to BIGINT to support WiseModel partner's large
// point values. SQLite uses dynamic INTEGER (already 64-bit) so no change
// is needed there.
//
// This mirrors the column-handling logic that lived in
// model.migrateWisemodelIntsToBigint before the Phase A refactor — the
// generic users.quota migration stayed on main as
// model.migrateUserQuotaToBigint, while the wisemodel_packages.* columns
// moved here.
func migrateWisemodelBigint() error {
	if common.UsingSQLite {
		return nil
	}

	type colSpec struct {
		table  string
		column string
	}
	cols := []colSpec{
		{"wisemodel_packages", "original_points"},
		{"wisemodel_packages", "original_tokens"},
	}

	for _, c := range cols {
		if !model.DB.Migrator().HasTable(c.table) {
			continue
		}

		var alterSQL string
		if common.UsingPostgreSQL {
			var dataType string
			if err := model.DB.Raw(
				`SELECT data_type FROM information_schema.columns
				 WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
				c.table, c.column).Scan(&dataType).Error; err != nil {
				common.SysLog(fmt.Sprintf("Warning: failed to query %s.%s type: %v", c.table, c.column, err))
				continue
			}
			if dataType == "" || dataType == "bigint" {
				continue
			}
			alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE bigint`, c.table, c.column)
		} else if common.UsingMySQL {
			var columnType string
			if err := model.DB.Raw(
				`SELECT COLUMN_TYPE FROM information_schema.columns
				 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
				c.table, c.column).Scan(&columnType).Error; err != nil {
				common.SysLog(fmt.Sprintf("Warning: failed to query %s.%s type: %v", c.table, c.column, err))
				continue
			}
			if columnType == "" || strings.HasPrefix(strings.ToLower(columnType), "bigint") {
				continue
			}
			alterSQL = fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN `%s` bigint NOT NULL DEFAULT 0", c.table, c.column)
		}

		if alterSQL != "" {
			if err := model.DB.Exec(alterSQL).Error; err != nil {
				return fmt.Errorf("failed to migrate %s.%s to bigint: %w", c.table, c.column, err)
			}
			common.SysLog(fmt.Sprintf("Migrated %s.%s to bigint", c.table, c.column))
		}
	}
	return nil
}
