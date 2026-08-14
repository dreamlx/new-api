package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Regression test for issue #12: a subscription_plans table created by a
// previous boot (raw DDL with decimal(10,6), see ensureSubscriptionPlanTableSQLite)
// must survive AutoMigrate on restart. glebarez/sqlite v1.9.0's AlterColumn
// mangled DDLs at the comma inside decimal(10,6), producing
// "invalid DDL, unbalanced brackets" and a fatal startup.
func TestSubscriptionPlanAutoMigrateOnPopulatedSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/test.db"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE `+"`subscription_plans`"+` (
`+"`id`"+` integer,
`+"`title`"+` varchar(128) NOT NULL,
`+"`price_amount`"+` decimal(10,6) NOT NULL,
`+"`currency`"+` varchar(8) NOT NULL DEFAULT 'USD',
`+"`enabled`"+` numeric DEFAULT 1,
`+"`created_at`"+` bigint,
`+"`updated_at`"+` bigint,
PRIMARY KEY (`+"`id`"+`)
)`).Error)

	// Second boot: AutoMigrate must parse the existing DDL without error.
	require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}))
	// Repeated migrations must stay stable.
	require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}))
}
