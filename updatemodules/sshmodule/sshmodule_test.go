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

package sshmodule_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	log "github.com/sirupsen/logrus"

	"aos_updatemanager/updatemodules/sshmodule"
)

/*******************************************************************************
 * Var
 ******************************************************************************/

var tmpDir string

/*******************************************************************************
 * Init
 ******************************************************************************/

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: false,
		TimestampFormat:  "2006-01-02 15:04:05.000",
		FullTimestamp:    true})
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
}

/*******************************************************************************
 * Main
 ******************************************************************************/

func TestMain(m *testing.M) {
	var err error

	tmpDir, err = ioutil.TempDir("", "um_")
	if err != nil {
		log.Fatalf("Error create temporary dir: %s", err)
	}

	ret := m.Run()

	if err := os.RemoveAll(tmpDir); err != nil {
		log.Fatalf("Error removing tmp dir: %s", err)
	}

	os.Exit(ret)
}

/*******************************************************************************
 * Tests
 ******************************************************************************/

func TestGetID(t *testing.T) {
	module, err := sshmodule.New("TestComponent", nil, nil)
	if err != nil {
		t.Fatalf("Can't create ssh module: %s", err)
	}
	defer module.Close()

	if module.GetID() != "TestComponent" {
		t.Errorf("Wrong module ID: %s", module.GetID())
	}
}

func TestUpdate(t *testing.T) {
	configJSON := `{
		"Host": "localhost:22",
		"User": "test",
		"Password": "test",
		"DestPath": "/tmp/remoteTestFile",
		"Commands":[
			"cd . ",
			"pwd",
			"ls"
		]
	}`

	module, err := sshmodule.New("TestComponent", []byte(configJSON), nil)
	if err != nil {
		log.Fatalf("Can't create ssh module: %s", err)
	}
	defer module.Close()

	imagePath := path.Join(tmpDir, "testfile")

	if err := ioutil.WriteFile(imagePath, []byte("This is test file"), 0644); err != nil {
		log.Fatalf("Can't write test file: %s", err)
	}

	if err := module.Prepare(imagePath, "", nil); err != nil {
		t.Errorf("Prepare failed: %s", err)
	}

	if _, err := module.Update(); err != nil {
		t.Errorf("Update failed: %s", err)
	}

	if _, err := module.Apply(); err != nil {
		t.Errorf("Apply failed: %s", err)
	}
}

func TestWrongJson(t *testing.T) {
	configJSON := `{
		Wrong json format
		]
	}`

	module, err := sshmodule.New("TestComponent", []byte(configJSON), nil)
	if err == nil {
		module.Close()
		log.Fatalf("Expecting error here")
	}
}

func TestUpdateErrors(t *testing.T) {
	// NOTE: test with nonexisting host
	configJSON := `{
		"Host": "localhst:22",
		"User": "test",
		"Password": "test",
		"DestPath": "/tmp/remoteTestFile",
		"Commands":[
			"cd . ",
			"pwd",
			"ls"
		]
	}`

	module, err := sshmodule.New("TestComponent", []byte(configJSON), nil)
	if err != nil {
		log.Fatalf("Error creating module %s", err)
	}
	defer module.Close()

	imagePath := path.Join(tmpDir, "testfile")

	if err := ioutil.WriteFile(imagePath, []byte("This is test file"), 0644); err != nil {
		log.Fatalf("Can't write test file: %s", err)
	}

	if err := module.Prepare(imagePath, "", nil); err != nil {
		t.Errorf("Prepare failed: %s", err)
	}

	if _, err := module.Update(); err == nil {
		t.Errorf("Error expected because of wrong address")
	}

	if _, err := module.Revert(); err != nil {
		t.Errorf("Reverts failed: %s", err)
	}
}

func TestUpdateWrongCommands(t *testing.T) {
	// NOTE: test with some wrong command
	configJSON := `{
		"Host": "localhost:22",
		"User": "test",
		"Password": "test",
		"DestPath": "/tmp/remoteTestFile",
		"Commands":[
			"cd . ",
			"some wrong command",
			"ls"
		]
	}`

	module, err := sshmodule.New("TestComponent", []byte(configJSON), nil)
	if err != nil {
		log.Fatalf("Error creating module %s", err)
	}
	defer module.Close()

	imagePath := path.Join(tmpDir, "testfile")

	if err := ioutil.WriteFile(imagePath, []byte("This is test file"), 0644); err != nil {
		log.Fatalf("Can't write test file: %s", err)
	}

	if err := module.Prepare(imagePath, "", nil); err != nil {
		t.Errorf("Prepare failed: %s", err)
	}

	// NOTE: amoi - leaving this test to be failed right now. runCommands should handle error.
	if _, err := module.Update(); err == nil {
		t.Errorf("Error expected because command set is wrong")
	}

	if _, err := module.Revert(); err != nil {
		t.Errorf("Reverts failed: %s", err)
	}
}
