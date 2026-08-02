// Command authz-concurrency-probe answers whether replacing one role's
// permission set stays atomic when two transactions do it at once, on each
// database the framework supports.
//
// A replacement is a delete followed by an insert:
//
//	DELETE FROM casbin_rule WHERE ptype='p' AND v0=<tenant> AND v1=<role>
//	INSERT INTO casbin_rule ...                    -- the whole new set
//
// Two of those interleaving can leave the union of both sets instead of the
// later one, which storage keeps and the deciding process never learns about.
// Whether they can interleave depends on what the surrounding transaction is
// already holding, and that differs per write path and per engine — which is
// what this probe measures rather than reasons about.
//
// The role step below is the difference between the framework's write paths:
//
//	written  Role.CreateAfter / Role.UpdateAfter — the role row is written
//	         first, so its exclusive lock is held for the rest of the
//	         transaction.
//	read     Menu.UpdateAfter before rolesToRefresh took a lock — the roles are
//	         read and nothing is held.
//	locked   Menu.UpdateAfter as it stands — the roles are read FOR UPDATE.
//
// Each of those is run against a role that already holds permissions and
// against one that holds none, because rows that do not exist cannot be locked
// and that turns out to decide the outcome.
//
// SQLite is not probed: it admits one writer at a time, so the interleaving
// this measures cannot occur there.
//
// Run it against the containers in testdata/mysql and testdata/postgresql:
//
//	go run ./testdata/authz-concurrency-probe
//
// It exits non-zero if any scenario unions.
package main

import (
	"fmt"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	policyTable = "probe_casbin_rule"
	roleTable   = "probe_roles"

	probeTenant = "default"
	probeRole   = "role_a"

	// blockTimeout is how long a statement may wait before the probe calls it
	// blocked. A lock wait is the outcome being measured, not a failure, so the
	// scenario stops there and reports it.
	blockTimeout = 3 * time.Second
)

type dialect struct {
	name string
	open func() gorm.Dialector
	ddl  []string
}

func dialects() []dialect {
	return []dialect{
		{
			name: "postgres",
			open: func() gorm.Dialector {
				return postgres.Open(
					"host=127.0.0.1 port=5432 user=test password=test dbname=test sslmode=disable",
				)
			},
			ddl: []string{
				`CREATE TABLE ` + policyTable + ` (
					id BIGSERIAL PRIMARY KEY,
					ptype VARCHAR(100) NOT NULL DEFAULT '',
					v0 VARCHAR(100) NOT NULL DEFAULT '', v1 VARCHAR(100) NOT NULL DEFAULT '',
					v2 VARCHAR(100) NOT NULL DEFAULT '', v3 VARCHAR(100) NOT NULL DEFAULT '',
					v4 VARCHAR(100) NOT NULL DEFAULT '', v5 VARCHAR(100) NOT NULL DEFAULT '',
					CONSTRAINT uq_probe UNIQUE (ptype, v0, v1, v2, v3, v4, v5))`,
				`CREATE TABLE ` + roleTable + ` (id VARCHAR(191) PRIMARY KEY, name VARCHAR(191))`,
			},
		},
		{
			name: "mysql",
			open: func() gorm.Dialector {
				return mysql.Open(
					"test:test@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local",
				)
			},
			ddl: []string{
				`CREATE TABLE ` + policyTable + ` (
					id BIGINT AUTO_INCREMENT PRIMARY KEY,
					ptype VARCHAR(100) NOT NULL DEFAULT '',
					v0 VARCHAR(100) NOT NULL DEFAULT '', v1 VARCHAR(100) NOT NULL DEFAULT '',
					v2 VARCHAR(100) NOT NULL DEFAULT '', v3 VARCHAR(100) NOT NULL DEFAULT '',
					v4 VARCHAR(100) NOT NULL DEFAULT '', v5 VARCHAR(100) NOT NULL DEFAULT '',
					UNIQUE KEY uq_probe (ptype, v0, v1, v2, v3, v4, v5)) ENGINE=InnoDB`,
				`CREATE TABLE ` + roleTable + ` (id VARCHAR(191) PRIMARY KEY, name VARCHAR(191)) ENGINE=InnoDB`,
			},
		},
	}
}

// roleStep is what a write path does to the role row before it rewrites that
// role's permissions.
type roleStep string

const (
	roleWritten roleStep = "written" // Role.CreateAfter / Role.UpdateAfter
	roleRead    roleStep = "read"    // Menu.UpdateAfter, before it took a lock
	roleLocked  roleStep = "locked"  // Menu.UpdateAfter, as it stands
)

func (s roleStep) run(tx *gorm.DB) error {
	switch s {
	case roleWritten:
		return tx.Exec("UPDATE "+roleTable+" SET name = ? WHERE id = ?", "renamed", probeRole).Error
	case roleLocked:
		var name string
		return tx.Raw("SELECT name FROM "+roleTable+" WHERE id = ? FOR UPDATE", probeRole).Scan(&name).Error
	default:
		var name string
		return tx.Raw("SELECT name FROM "+roleTable+" WHERE id = ?", probeRole).Scan(&name).Error
	}
}

func clearSet(tx *gorm.DB) error {
	return tx.Exec(
		"DELETE FROM "+policyTable+" WHERE ptype = ? AND v0 = ? AND v1 = ?", "p", probeTenant, probeRole,
	).Error
}

func installSet(objects ...string) func(*gorm.DB) error {
	return func(tx *gorm.DB) error {
		for _, object := range objects {
			err := tx.Exec(
				"INSERT INTO "+policyTable+" (ptype, v0, v1, v2, v3, v4, v5) VALUES (?, ?, ?, ?, ?, ?, ?)",
				"p", probeTenant, probeRole, object, "GET", "allow", "",
			).Error
			if err != nil {
				return err
			}
		}
		return nil
	}
}

type step struct {
	label string
	tx    *gorm.DB
	run   func(*gorm.DB) error
}

type outcome struct {
	verdict string
	rows    []string
	log     []string
}

// run drives two transactions through the statement order that models two
// administrators saving the same role at once, and reports what the policy
// table holds afterwards.
func run(db *gorm.DB, role roleStep, seeded bool) (outcome, error) {
	if err := db.Exec("DELETE FROM " + policyTable).Error; err != nil {
		return outcome{}, err
	}
	if seeded {
		if err := installSet("/api/old1", "/api/old2")(db); err != nil {
			return outcome{}, err
		}
	}

	t1, t2 := db.Begin(), db.Begin()
	defer func() {
		t1.Rollback()
		t2.Rollback()
	}()

	var result outcome
	for _, s := range []step{
		{"T1 role", t1, role.run},
		{"T2 role", t2, role.run},
		{"T1 DELETE", t1, clearSet},
		{"T2 DELETE", t2, clearSet},
		{"T1 INSERT A", t1, installSet("/api/a1", "/api/a2")},
		{"T1 COMMIT", t1, func(tx *gorm.DB) error { return tx.Commit().Error }},
		{"T2 INSERT B", t2, installSet("/api/b1", "/api/b2")},
		{"T2 COMMIT", t2, func(tx *gorm.DB) error { return tx.Commit().Error }},
	} {
		done := make(chan error, 1)
		begin := time.Now()
		go func() { done <- s.run(s.tx) }()

		select {
		case err := <-done:
			status := "ok"
			if err != nil {
				status = "ERR: " + err.Error()
			}
			result.log = append(result.log,
				fmt.Sprintf("      %-12s %-6s %s", s.label, time.Since(begin).Round(time.Millisecond), status))
		case <-time.After(blockTimeout):
			result.log = append(result.log, fmt.Sprintf("      %-12s BLOCKED, waiting on a lock", s.label))
			result.verdict = "SERIALIZED by lock"
			return result, nil
		}
	}

	err := db.Raw(
		"SELECT v2 FROM "+policyTable+" WHERE ptype = ? AND v0 = ? AND v1 = ? ORDER BY v2",
		"p", probeTenant, probeRole,
	).Scan(&result.rows).Error
	if err != nil {
		return result, err
	}

	result.verdict = "REPLACED"
	if len(result.rows) > 2 {
		result.verdict = "UNION"
	}
	return result, nil
}

func probe(d dialect) (unions int) {
	fmt.Printf("== %s ==\n", d.name)
	db, err := gorm.Open(d.open(), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		fmt.Printf("   connect failed: %v\n\n", err)
		return 0
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Printf("   no connection pool: %v\n\n", err)
		return 0
	}
	defer sqlDB.Close()
	// Each transaction needs a connection of its own, or the second waits for
	// the first to release the only one and nothing ever interleaves.
	sqlDB.SetMaxOpenConns(8)

	defer dropTables(db)
	dropTables(db)
	for _, ddl := range d.ddl {
		if err := db.Exec(ddl).Error; err != nil {
			fmt.Printf("   create table failed: %v\n\n", err)
			return 0
		}
	}
	db.Exec("INSERT INTO "+roleTable+" (id, name) VALUES (?, ?)", probeRole, "Role A")

	for _, role := range []roleStep{roleWritten, roleRead, roleLocked} {
		for _, seeded := range []bool{false, true} {
			held := "role holds no permissions"
			if seeded {
				held = "role already holds permissions"
			}
			result, err := run(db, role, seeded)
			if err != nil {
				fmt.Printf("   role=%-8s %-30s FAILED: %v\n", role, held, err)
				continue
			}
			if result.verdict == "UNION" {
				unions++
			}
			fmt.Printf("   role=%-8s %-30s -> %-18s %v\n", role, held, result.verdict, result.rows)
			for _, line := range result.log {
				fmt.Println(line)
			}
		}
	}
	fmt.Println()
	return unions
}

func dropTables(db *gorm.DB) {
	db.Exec("DROP TABLE IF EXISTS " + policyTable)
	db.Exec("DROP TABLE IF EXISTS " + roleTable)
}

func main() {
	unions := 0
	for _, d := range dialects() {
		unions += probe(d)
	}

	if unions > 0 {
		fmt.Printf("RESULT: %d scenario(s) unioned — those write paths are not serialized.\n", unions)
		os.Exit(1)
	}
	fmt.Println("RESULT: no scenario unioned.")
}
