// SPDX-License-Identifier: Apache-2.0
//
// Copyright (C) 2021 Renesas Electronics Corporation.
// Copyright (C) 2021 EPAM Systems, Inc.
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

package overlaymodule

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"regexp"

	log "github.com/sirupsen/logrus"
	"gitpct.epam.com/epmd-aepr/aos_common/aoserrors"

	"aos_updatemanager/database"
	"aos_updatemanager/updatehandler"
)

// Success update sequence diagram:
//
// Prepare(path)  -> set state "prepared"
// Update()       -> set requestReboot
// Reboot()       -> requestReboot is set, perform system reboot
//------------------------------- Reboot ---------------------------------------
// Init()         -> boot OK, set state "updated", clear requestReboot
// Update()       -> return OK, already in "updated" state
// Reboot()       -> return OK, requestReboot is not set
// Apply()        -> set requestReboot
// Reboot()       -> requestReboot is set, perform system reboot
//------------------------------- Reboot ---------------------------------------
// Init()         -> if boot OK, set state "idle", clear requestReboot
// Apply()        -> return OK, already in "idle" state
// Reboot()       -> return OK, requestReboot is not set

// Failed update sequence diagram:
//
// Prepare(path)  -> set state "prepared"
// Update()       -> set requestReboot
// Reboot()       -> requestReboot is set, perform system reboot
//------------------------------- Reboot ---------------------------------------
// Init()         -> boot not OK, set update error, clear requestReboot
// Update()       -> return update error
// Revert()       -> set state idle
// Reboot()       -> retrun OK, requestReboot is not set

/*******************************************************************************
 * Consts
 ******************************************************************************/

const (
	idleState = iota
	preparedState
	updatedState
)

const (
	doUpdateFileName = "do_update"
	doApplyFileName  = "do_apply"
	updatedFileName  = "updated"
	failedFileName   = "failed"
	imageExtension   = ".squashfs"
)

/*******************************************************************************
 * Types
 ******************************************************************************/

// OverlayModule overlay module
type OverlayModule struct {
	id             string
	config         moduleConfig
	storage        updatehandler.ModuleStorage
	state          moduleState
	bootWithUpdate bool
	bootFailed     bool
	rebooter       Rebooter
	vendorVersion  string
}

// Rebooter performs module reboot
type Rebooter interface {
	Reboot() (err error)
}

type moduleState struct {
	UpdateState   updateState `json:"updateState"`
	RebootRequest bool        `json:"rebootRequired"`
	UpdateType    string      `json:"updateType"`
}

type updateState int

type moduleMetadata struct {
	Type string `json:"type"`
}

type moduleConfig struct {
	VersionFile string `json:"versionFile"`
	UpdateDir   string `json:"updateDir"`
}

/*******************************************************************************
 * Public
 ******************************************************************************/

// New creates module instance
func New(id string, configJSON json.RawMessage,
	storage updatehandler.ModuleStorage, rebooter Rebooter) (module updatehandler.UpdateModule, err error) {
	log.WithFields(log.Fields{"id": id}).Debug("Create overlay module")

	if storage == nil {
		return nil, aoserrors.New("no storage provided")
	}

	overlayModule := &OverlayModule{id: id, storage: storage, rebooter: rebooter}

	if err = json.Unmarshal(configJSON, &overlayModule.config); err != nil {
		return nil, aoserrors.Wrap(err)
	}

	if overlayModule.config.VersionFile == "" {
		return nil, aoserrors.New("version file is not set")
	}

	if overlayModule.config.UpdateDir == "" {
		return nil, aoserrors.New("update dir is nit set")
	}

	if err = overlayModule.getState(); err != nil {
		return nil, aoserrors.Wrap(err)
	}

	return overlayModule, nil
}

// Close closes module
func (module *OverlayModule) Close() (err error) {
	log.WithField("id", module.id).Debug("Close overlay module")

	return nil
}

// Init initializes module
func (module *OverlayModule) Init() (err error) {
	log.WithFields(log.Fields{"id": module.id}).Debug("Init overlay module")

	if module.vendorVersion, err = module.getModuleVersion(); err != nil {
		return aoserrors.Wrap(err)
	}

	if module.state.RebootRequest {
		module.state.RebootRequest = false

		if err = module.saveState(); err != nil {
			return aoserrors.Wrap(err)
		}
	}

	if module.state.UpdateState == idleState {
		return
	}

	updatedFile := path.Join(module.config.UpdateDir, updatedFileName)

	if _, err = os.Stat(updatedFile); err == nil {
		module.bootWithUpdate = true
	}

	failedFile := path.Join(module.config.UpdateDir, failedFileName)

	if _, err = os.Stat(failedFile); err == nil {
		module.bootFailed = true

		if err = os.Remove(failedFile); err != nil {
			return aoserrors.Wrap(err)
		}

	}

	return nil
}

// GetID returns module ID
func (module *OverlayModule) GetID() (id string) {
	return module.id
}

// GetVendorVersion returns vendor version
func (module *OverlayModule) GetVendorVersion() (version string, err error) {
	return module.vendorVersion, nil
}

// Prepare prepares module update
func (module *OverlayModule) Prepare(imagePath string, vendorVersion string, annotations json.RawMessage) (err error) {
	log.WithFields(log.Fields{
		"id":            module.id,
		"imagePath":     imagePath,
		"vendorVersion": vendorVersion}).Debug("Prepare overlay module")

	if module.state.UpdateState != idleState && module.state.UpdateState != preparedState {
		return aoserrors.Errorf("wrong state: %s", module.state.UpdateState)
	}

	var metadata moduleMetadata

	if err = json.Unmarshal(annotations, &metadata); err != nil {
		return aoserrors.Wrap(err)
	}

	module.state.UpdateType = metadata.Type
	module.state.UpdateState = preparedState

	if err = module.clearUpdateDir(); err != nil {
		return aoserrors.Wrap(err)
	}

	if err = os.Rename(imagePath, path.Join(module.config.UpdateDir, path.Base(imagePath)+imageExtension)); err != nil {
		return aoserrors.Wrap(err)
	}

	if err = module.saveState(); err != nil {
		return aoserrors.Wrap(err)
	}

	return nil
}

// Update performs module update
func (module *OverlayModule) Update() (rebootRequired bool, err error) {
	log.WithFields(log.Fields{"id": module.id}).Debug("Update overlay module")

	if module.state.UpdateState == updatedState {
		if !module.bootWithUpdate {
			return false, aoserrors.New("boot with update failed")
		}

		return false, nil
	}

	if module.state.UpdateState != preparedState {
		return false, aoserrors.Errorf("wrong state: %s", module.state.UpdateState)
	}

	if err = ioutil.WriteFile(path.Join(module.config.UpdateDir, doUpdateFileName),
		[]byte(module.state.UpdateType), 0644); err != nil {
		return false, aoserrors.Wrap(err)
	}

	module.state.UpdateState = updatedState
	module.state.RebootRequest = true

	if err = module.saveState(); err != nil {
		return false, aoserrors.Wrap(err)
	}

	return module.state.RebootRequest, nil
}

// Apply applies current update
func (module *OverlayModule) Apply() (rebootRequired bool, err error) {
	log.WithFields(log.Fields{"id": module.id}).Debug("Apply overlay module")

	// Remove updated flag
	os.Remove(path.Join(module.config.UpdateDir, updatedFileName))

	if module.bootFailed {
		return false, aoserrors.New("current boot failed")
	}

	if module.state.UpdateState == idleState {
		if err = module.clearUpdateDir(); err != nil {
			return false, aoserrors.Wrap(err)
		}

		return false, nil
	}

	if module.state.UpdateState != updatedState {
		return false, aoserrors.Errorf("wrong state: %s", module.state.UpdateState)
	}

	if err = ioutil.WriteFile(path.Join(module.config.UpdateDir, doApplyFileName),
		[]byte(module.state.UpdateType), 0644); err != nil {
		return false, aoserrors.Wrap(err)
	}

	module.state.UpdateState = idleState
	module.state.RebootRequest = true

	if err = module.saveState(); err != nil {
		return false, aoserrors.Wrap(err)
	}

	return module.state.RebootRequest, nil
}

// Revert reverts current update
func (module *OverlayModule) Revert() (rebootRequired bool, err error) {
	log.WithFields(log.Fields{"id": module.id}).Debug("Revert overlay module")

	// Remove updated flag
	os.Remove(path.Join(module.config.UpdateDir, updatedFileName))

	if module.state.UpdateState == idleState {
		return false, nil
	}

	if err = module.clearUpdateDir(); err != nil {
		return false, aoserrors.Wrap(err)
	}

	if module.bootWithUpdate {
		module.state.RebootRequest = true
	}

	module.state.UpdateState = idleState

	if err = module.saveState(); err != nil {
		return false, aoserrors.Wrap(err)
	}

	return module.state.RebootRequest, nil
}

// Reboot performs module reboot
func (module *OverlayModule) Reboot() (err error) {
	if module.rebooter != nil && module.state.RebootRequest {
		log.WithFields(log.Fields{"id": module.id}).Debug("Reboot overlay module")

		if err = module.rebooter.Reboot(); err != nil {
			return aoserrors.Wrap(err)
		}
	}

	return nil
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (state updateState) String() string {
	return [...]string{"idle", "prepared", "updated"}[state]
}

func (module *OverlayModule) saveState() (err error) {
	log.WithFields(log.Fields{"id": module.id, "state": module.state.UpdateState}).Debug("Save state")

	stateJSON, err := json.Marshal(module.state)
	if err != nil {
		return aoserrors.Wrap(err)
	}

	if err = module.storage.SetModuleState(module.id, stateJSON); err != nil {
		return aoserrors.Wrap(err)
	}

	return nil
}

func (module *OverlayModule) getState() (err error) {
	stateJSON, err := module.storage.GetModuleState(module.id)
	if err != nil {
		if err == database.ErrNotExist {
			module.state = moduleState{}

			return nil
		}

		return aoserrors.Wrap(err)
	}

	if stateJSON != nil {
		if err = json.Unmarshal(stateJSON, &module.state); err != nil {
			return aoserrors.Wrap(err)
		}
	}

	log.WithFields(log.Fields{"id": module.id, "state": module.state.UpdateState}).Debug("Get state")

	return nil
}

func (module *OverlayModule) getModuleVersion() (version string, err error) {
	data, err := ioutil.ReadFile(module.config.VersionFile)
	if err != nil {
		return "", aoserrors.Errorf("nonexistent or empty vendor version file %s, err: %s", module.config.VersionFile, err)
	}

	pattern := regexp.MustCompile(`VERSION\s*=\s*\"(.+)\"`)

	loc := pattern.FindSubmatchIndex(data)
	if loc == nil {
		return "", aoserrors.Errorf("vendor version file has wrong format")
	}

	return string(data[loc[2]:loc[3]]), nil
}

func (module *OverlayModule) clearUpdateDir() (err error) {
	if err = os.RemoveAll(module.config.UpdateDir); err != nil {
		return aoserrors.Wrap(err)
	}

	if err = os.MkdirAll(module.config.UpdateDir, 0755); err != nil {
		return aoserrors.Wrap(err)
	}

	return nil
}
