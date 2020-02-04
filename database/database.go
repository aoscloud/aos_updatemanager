// SPDX-License-Identifier: Apache-2.0
//
// Copyright 2019 Renesas Inc.
// Copyright 2019 EPAM Systems Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package database

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" //ignore lint
	log "github.com/sirupsen/logrus"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

const (
	dbVersion = 2
)

/*******************************************************************************
 * Vars
 ******************************************************************************/

// ErrNotExist is returned when requested entry not exist in DB
var ErrNotExist = errors.New("entry doesn't not exist")

// ErrVersionMismatch is returned when DB has unsupported DB version
var ErrVersionMismatch = errors.New("version mismatch")

/*******************************************************************************
 * Types
 ******************************************************************************/

// Database structure with database information
type Database struct {
	sql *sql.DB
}

/*******************************************************************************
 * Public
 ******************************************************************************/

// New creates new database handle
func New(name string) (db *Database, err error) {
	log.WithField("name", name).Debug("Open database")

	// Check and create db path
	if _, err = os.Stat(filepath.Dir(name)); err != nil {
		if !os.IsNotExist(err) {
			return db, err
		}
		if err = os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return db, err
		}
	}

	var sqlite *sql.DB

	sqlite, err = sql.Open("sqlite3", name)
	if err != nil {
		return db, err
	}

	defer func() {
		if err != nil {
			sqlite.Close()
		}
	}()

	db = &Database{sqlite}

	if err = db.createConfigTable(); err != nil {
		return db, err
	}

	if err = db.createModuleStatusesTable(); err != nil {
		return db, err
	}

	version, err := db.getVersion()
	if err != nil {
		return db, err
	}

	if version != dbVersion {
		return db, ErrVersionMismatch
	}

	return db, nil
}

// SetState stores state
func (db *Database) SetState(state int) (err error) {
	result, err := db.sql.Exec("UPDATE config SET state = ?", state)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotExist
	}

	return nil
}

// GetState returns state
func (db *Database) GetState() (state int, err error) {
	stmt, err := db.sql.Prepare("SELECT state FROM config")
	if err != nil {
		return state, err
	}
	defer stmt.Close()

	err = stmt.QueryRow().Scan(&state)
	if err != nil {
		if err == sql.ErrNoRows {
			return state, ErrNotExist
		}

		return state, err
	}

	return state, nil
}

// SetOperationStage stores operation stage
func (db *Database) SetOperationStage(stage int) (err error) {
	result, err := db.sql.Exec("UPDATE config SET stage = ?", stage)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotExist
	}

	return nil
}

// GetOperationStage returns operation stage
func (db *Database) GetOperationStage() (stage int, err error) {
	stmt, err := db.sql.Prepare("SELECT stage FROM config")
	if err != nil {
		return stage, err
	}
	defer stmt.Close()

	err = stmt.QueryRow().Scan(&stage)
	if err != nil {
		if err == sql.ErrNoRows {
			return stage, ErrNotExist
		}

		return stage, err
	}

	return stage, nil
}

// SetOperationVersion stores operation version
func (db *Database) SetOperationVersion(version uint64) (err error) {
	result, err := db.sql.Exec("UPDATE config SET operationVersion = ?", version)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotExist
	}

	return nil
}

// GetOperationVersion returns upgrade version
func (db *Database) GetOperationVersion() (version uint64, err error) {
	stmt, err := db.sql.Prepare("SELECT operationVersion FROM config")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	if err = stmt.QueryRow().Scan(&version); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNotExist
		}

		return 0, err
	}

	return version, nil
}

// SetCurrentVersion stores current image version
func (db *Database) SetCurrentVersion(version uint64) (err error) {
	result, err := db.sql.Exec("UPDATE config SET currentVersion = ?", version)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotExist
	}

	return nil
}

// GetCurrentVersion returns current image version
func (db *Database) GetCurrentVersion() (version uint64, err error) {
	stmt, err := db.sql.Prepare("SELECT currentVersion FROM config")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	if err = stmt.QueryRow().Scan(&version); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNotExist
		}

		return 0, err
	}

	return version, nil
}

// SetImagePath stores image path
func (db *Database) SetImagePath(path string) (err error) {
	result, err := db.sql.Exec("UPDATE config SET imagePath = ?", path)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotExist
	}

	return nil
}

// GetImagePath returns image path
func (db *Database) GetImagePath() (path string, err error) {
	stmt, err := db.sql.Prepare("SELECT imagePath FROM config")
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	if err = stmt.QueryRow().Scan(&path); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotExist
		}

		return "", err
	}

	return path, nil
}

// SetLastError stores last error
func (db *Database) SetLastError(lastError error) (err error) {
	errorStr := ""

	if lastError != nil {
		errorStr = lastError.Error()
	}

	result, err := db.sql.Exec("UPDATE config SET lastError = ?", errorStr)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotExist
	}

	return nil
}

// GetLastError returns last error
func (db *Database) GetLastError() (lastError error, err error) {
	stmt, err := db.sql.Prepare("SELECT lastError FROM config")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	errorStr := ""

	if err = stmt.QueryRow().Scan(&errorStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotExist
		}

		return nil, err
	}

	if errorStr == "" {
		return nil, nil
	}

	return errors.New(errorStr), nil
}

// AddModuleStatus adds module status to DB
func (db *Database) AddModuleStatus(id string, status error) (err error) {
	statusStr := ""

	if status != nil {
		statusStr = status.Error()
	}

	if _, err = db.sql.Exec("INSERT INTO moduleStatuses VALUES(?, ?)", id, statusStr); err != nil {
		return err
	}

	return nil
}

// RemoveModuleStatus removes module status from DB
func (db *Database) RemoveModuleStatus(id string) (err error) {
	if _, err = db.sql.Exec("DELETE FROM moduleStatuses WHERE module = ?", id); err != nil {
		return err
	}

	return nil
}

// GetModuleStatuses returns all added module statuses
func (db *Database) GetModuleStatuses() (moduleStatuses map[string]error, err error) {
	rows, err := db.sql.Query("SELECT * FROM moduleStatuses")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	moduleStatuses = make(map[string]error)

	for rows.Next() {
		var id, statusStr string

		if err = rows.Scan(&id, &statusStr); err != nil {
			return nil, err
		}

		var status error

		if statusStr != "" {
			status = errors.New(statusStr)
		}

		moduleStatuses[id] = status
	}

	return moduleStatuses, rows.Err()
}

// ClearModuleStatuses clears module statuses DB
func (db *Database) ClearModuleStatuses() (err error) {
	_, err = db.sql.Exec("DELETE FROM moduleStatuses")

	return err
}

// Close closes database
func (db *Database) Close() {
	db.sql.Close()
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (db *Database) getVersion() (version uint64, err error) {
	stmt, err := db.sql.Prepare("SELECT version FROM config")
	if err != nil {
		return version, err
	}
	defer stmt.Close()

	err = stmt.QueryRow().Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return version, ErrNotExist
		}

		return version, err
	}

	return version, nil
}

func (db *Database) setVersion(version uint64) (err error) {
	result, err := db.sql.Exec("UPDATE config SET version = ?", version)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNotExist
	}

	return nil
}

func (db *Database) isTableExist(name string) (result bool, err error) {
	rows, err := db.sql.Query("SELECT * FROM sqlite_master WHERE name = ? and type='table'", name)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	result = rows.Next()

	return result, rows.Err()
}

func (db *Database) createConfigTable() (err error) {
	log.Info("Create config table")

	exist, err := db.isTableExist("config")
	if err != nil {
		return err
	}

	if exist {
		return nil
	}

	if _, err = db.sql.Exec(
		`CREATE TABLE config (
			version INTEGER,
			state INTEGER,
			stage INTEGER,
			currentVersion INTEGER,
			operationVersion INTEGER,
			imagePath TEXT,
			lastError TEXT)`); err != nil {
		return err
	}

	if _, err = db.sql.Exec(
		`INSERT INTO config (
			version,
			state,
			stage,
			currentVersion,
			operationVersion,
			imagePath,
			lastError) values(?, ?, ?, ?, ?, ?, ?)`, dbVersion, 0, 0, 0, 0, "", ""); err != nil {
		return err
	}

	return nil
}

func (db *Database) createModuleStatusesTable() (err error) {
	log.Info("Create module status table")

	exist, err := db.isTableExist("moduleStatuses")
	if err != nil {
		return err
	}

	if exist {
		return nil
	}

	if _, err = db.sql.Exec(
		`CREATE TABLE moduleStatuses (
			module TEXT NOT NULL PRIMARY KEY,
			status TEXT)`); err != nil {
		return err
	}

	return nil
}
