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

package updatehandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"

	log "github.com/sirupsen/logrus"
	"gitpct.epam.com/nunc-ota/aos_common/image"
	"gitpct.epam.com/nunc-ota/aos_common/umprotocol"

	"aos_updatemanager/config"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

// Operations type
const (
	upgradeOperation = iota
	revertOperation
)

//
// Upgrade state machine:
//
// idleState          -> upgrade request                     -> initState
// initState          -> unpack image                        -> upgradeState
// upgradeState       -> upgrade modules                     -> finishState
// finishState        -> send status                         -> idleState
//
// If some error happens during upgrade modules:
//
// upgradeState       -> upgrade modules fails               -> cancelState
// cancelUpgradeState -> cancel upgrade modules, send status -> idleState
//
// Revert state machine:
//
// idleState          -> revert request                      -> initState
// initState          ->                                     -> revertState
// revertState        -> revert modules                      -> finishState
// finishState        -> send status                         -> idleState
//
// If some error happens during revert modules:
//
// revertState        -> revert modules fails                -> cancelState
// cancelRevertState  -> cancel revert modules, send status  -> idleState
//

const (
	idleState = iota
	initState
	upgradeState
	revertState
	cancelState
	finishState
)

// Module states
const (
	invalidState = iota
	upgradingState
	upgradedState
	revertingState
	revertedState
	cancelingState
	canceledState
)

const bundleDir = "bundleDir"
const metadataFileName = "metadata.json"

const statusChannelSize = 1

/*******************************************************************************
 * Types
 ******************************************************************************/

// Handler update handler
type Handler struct {
	sync.Mutex
	platform PlatformController
	storage  StateStorage
	modules  map[string]UpdateModule

	upgradeDir     string
	bundleDir      string
	state          handlerState
	currentVersion uint64

	wg sync.WaitGroup

	statusChannel chan string
}

// UpdateModule interface for module plugin
type UpdateModule interface {
	// GetID returns module ID
	GetID() (id string)
	// Init initializes module
	Init() (err error)
	// Upgrade upgrades module
	Upgrade(version uint64, imagePath string) (rebootRequired bool, err error)
	// CancelUpgrade cancels upgrade
	CancelUpgrade(version uint64) (rebootRequired bool, err error)
	// FinishUpgrade finished upgrade
	FinishUpgrade(version uint64) (err error)
	// Revert reverts module
	Revert(version uint64) (rebootRequired bool, err error)
	// CancelRevert cancels revert module
	CancelRevert(version uint64) (rebootRequired bool, err error)
	// FinishRevert finished revert
	FinishRevert(version uint64) (err error)
	// Close closes update module
	Close() (err error)
}

// StateStorage provides API to store/retreive persistent data
type StateStorage interface {
	SetOperationState(jsonState []byte) (err error)
	GetOperationState() (jsonState []byte, err error)
}

// PlatformController platform controller
type PlatformController interface {
	GetVersion() (version uint64, err error)
	SetVersion(version uint64) (err error)
	GetPlatformID() (id string, err error)
	SystemReboot() (err error)
}

type handlerState struct {
	OperationType    operationType          `json:"operationType"`
	OperationState   operationState         `json:"operationState"`
	OperationVersion uint64                 `json:"operationVersion"`
	ImagePath        string                 `json:"imagePath"`
	LastError        error                  `json:"-"`
	ErrorMsg         string                 `json:"errorMsg"`
	ModuleStates     map[string]moduleState `json:"moduleStates"`
}

type itemMetadata struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type imageMetadata struct {
	PlatformID        string         `json:"platformId"`
	BundleVersion     string         `json:"bundleVersion,omitempty"`
	BundleDescription string         `json:"bundleDescription,omitempty"`
	UpdateItems       []itemMetadata `json:"updateItems"`
}

type operationType int
type operationState int
type moduleState int

/*******************************************************************************
 * Public
 ******************************************************************************/

// New returns pointer to new Handler
func New(cfg *config.Config, modules []UpdateModule,
	platform PlatformController, storage StateStorage) (handler *Handler, err error) {
	handler = &Handler{
		platform:      platform,
		storage:       storage,
		upgradeDir:    cfg.UpgradeDir,
		bundleDir:     path.Join(cfg.UpgradeDir, bundleDir),
		statusChannel: make(chan string, statusChannelSize),
	}

	if _, err := os.Stat(handler.bundleDir); os.IsNotExist(err) {
		if errMkdir := os.MkdirAll(handler.bundleDir, 0755); errMkdir != nil {
			return nil, errMkdir
		}
	}

	if handler.currentVersion, err = handler.platform.GetVersion(); err != nil {
		return nil, err
	}

	log.WithField("imageVersion", handler.currentVersion).Debug("Create update handler")

	if err = handler.getState(); err != nil {
		return nil, err
	}

	handler.modules = make(map[string]UpdateModule)

	for _, module := range modules {
		handler.modules[module.GetID()] = module
	}

	handler.wg.Add(1)
	go handler.init()

	return handler, nil
}

// GetCurrentVersion returns current system version
func (handler *Handler) GetCurrentVersion() (version uint64) {
	handler.Lock()
	defer handler.Unlock()

	return handler.currentVersion
}

// GetOperationVersion returns upgrade/revert version
func (handler *Handler) GetOperationVersion() (version uint64) {
	handler.Lock()
	defer handler.Unlock()

	return handler.state.OperationVersion
}

// GetStatus returns update status
func (handler *Handler) GetStatus() (status string) {
	handler.Lock()
	defer handler.Unlock()

	status = umprotocol.SuccessStatus

	if handler.state.OperationState != idleState {
		status = umprotocol.InProgressStatus
	}

	if handler.state.LastError != nil {
		status = umprotocol.FailedStatus
	}

	return status
}

// GetLastOperation returns last operation
func (handler *Handler) GetLastOperation() (operation string) {
	handler.Lock()
	defer handler.Unlock()

	if handler.state.OperationType == revertOperation {
		operation = umprotocol.RevertOperation
	}

	if handler.state.OperationType == upgradeOperation {
		operation = umprotocol.UpgradeOperation
	}

	return operation
}

// GetLastError returns last upgrade error
func (handler *Handler) GetLastError() (err error) {
	handler.Lock()
	defer handler.Unlock()

	return handler.state.LastError
}

// Upgrade performs upgrade operation
func (handler *Handler) Upgrade(version uint64, imageInfo umprotocol.ImageInfo) (err error) {
	handler.Lock()
	defer handler.Unlock()

	handler.wg.Wait()

	log.WithField("version", version).Info("Upgrade")

	if version <= handler.currentVersion {
		return fmt.Errorf("wrong upgrade version: %d", version)
	}

	if handler.state.OperationState != idleState {
		return errors.New("wrong state")
	}

	imagePath := path.Join(handler.upgradeDir, imageInfo.Path)

	if err = image.CheckFileInfo(imagePath, image.FileInfo{
		Sha256: imageInfo.Sha256,
		Sha512: imageInfo.Sha512,
		Size:   imageInfo.Size}); err != nil {
		return err
	}

	handler.state.OperationState = initState
	handler.state.OperationVersion = version
	handler.state.OperationType = upgradeOperation
	handler.state.LastError = nil
	handler.state.ImagePath = imagePath
	handler.state.ModuleStates = make(map[string]moduleState)

	if err = handler.setState(); err != nil {
		return err
	}

	go handler.upgrade()

	return nil
}

// Revert performs revert operation
func (handler *Handler) Revert(version uint64) (err error) {
	handler.Lock()
	defer handler.Unlock()

	handler.wg.Wait()

	log.WithField("version", version).Info("Revert")

	if version >= handler.currentVersion {
		return fmt.Errorf("wrong revert version: %d", version)
	}

	handler.state.OperationState = initState
	handler.state.OperationVersion = version
	handler.state.OperationType = revertOperation
	handler.state.LastError = nil

	if err = handler.setState(); err != nil {
		return err
	}

	go handler.revert()

	return nil
}

// StatusChannel this channel is used to notify when upgrade/revert is finished
func (handler *Handler) StatusChannel() (statusChannel <-chan string) {
	return handler.statusChannel
}

// Close closes update handler
func (handler *Handler) Close() {
	log.Debug("Close update handler")

	close(handler.statusChannel)
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (opType operationType) String() string {
	return [...]string{
		"upgrade", "revert"}[opType]
}

func (state operationState) String() string {
	return [...]string{
		"idle", "init", "upgrade", "revert", "cancel", "finish"}[state]
}

func (state moduleState) String() string {
	return [...]string{
		"invalid", "upgrading", "upgraded", "reverting", "reverted", "canceling", "canceled"}[state]
}

func (handler *Handler) getState() (err error) {
	jsonState, err := handler.storage.GetOperationState()
	if err != nil {
		return err
	}

	if err = json.Unmarshal(jsonState, &handler.state); err != nil {
		return err
	}

	if handler.state.ErrorMsg != "" {
		handler.state.LastError = errors.New(handler.state.ErrorMsg)
	} else {
		handler.state.LastError = nil
	}

	return nil
}

func (handler *Handler) setState() (err error) {
	if handler.state.LastError != nil {
		handler.state.ErrorMsg = handler.state.LastError.Error()
	} else {
		handler.state.ErrorMsg = ""
	}

	jsonState, err := json.Marshal(handler.state)
	if err != nil {
		return err
	}

	if err = handler.storage.SetOperationState(jsonState); err != nil {
		return err
	}

	return nil
}

func (handler *Handler) init() {
	defer handler.wg.Done()

	for id, module := range handler.modules {
		log.Debugf("Initializing module %s", id)

		if err := module.Init(); err != nil {
			log.Errorf("Can't initialize module %s: %s", id, err)
		}
	}

	if handler.state.OperationState != idleState {
		switch {
		case handler.state.OperationType == upgradeOperation:
			go handler.upgrade()

		case handler.state.OperationType == revertOperation:
			go handler.revert()
		}
	}
}

func (handler *Handler) finishOperation(operationError error) {
	handler.Lock()
	defer handler.Unlock()

	if operationError != nil {
		log.Errorf("Operation %s failed: %s", handler.state.OperationType, operationError)
	} else {
		log.Infof("Operation %s successfully finished", handler.state.OperationType)

		handler.currentVersion = handler.state.OperationVersion
	}

	if err := handler.platform.SetVersion(handler.currentVersion); err != nil {
		log.Errorf("Can't set current version: %s", err)
	}

	handler.state.OperationState = idleState
	handler.state.LastError = operationError

	if err := handler.setState(); err != nil {
		log.Errorf("Can't save update handler state: %s", err)
	}

	status := umprotocol.SuccessStatus

	if handler.state.LastError != nil {
		status = umprotocol.FailedStatus
	}

	handler.statusChannel <- status
}

func (handler *Handler) upgrade() {
	var (
		rebootRequired bool
		err            error
	)

	for {
		switch handler.state.OperationState {
		case initState:
			if err = handler.unpackImage(); err != nil {
				handler.switchState(finishState, err)
				break
			}

			handler.switchState(upgradeState, nil)

		case upgradeState:
			rebootRequired, err = handler.upgradeModules()
			if err != nil {
				handler.switchState(cancelState, err)
				break
			}

			if rebootRequired {
				break
			}

			handler.switchState(finishState, nil)

		case cancelState:
			rebootRequired, err = handler.cancelUpgradeModules()
			if err != nil {
				log.Errorf("Error canceling upgrade modules: %s", err)
			}

			if rebootRequired {
				break
			}

			handler.finishOperation(handler.state.LastError)

			return

		case finishState:
			if err = handler.finishUpgradeModules(); err != nil {
				log.Errorf("Error finishing upgrade modules: %s", err)
			}

			handler.finishOperation(err)

			return
		}

		if rebootRequired {
			log.Debug("System reboot is required")

			if err = handler.platform.SystemReboot(); err != nil {
				log.Errorf("Can't perform system reboot: %s", err)
			}

			return
		}
	}
}

func (handler *Handler) revert() {
	var (
		rebootRequired bool
		err            error
	)

	for {
		switch handler.state.OperationState {
		case initState:
			handler.switchState(revertState, nil)

		case revertState:
			rebootRequired, err = handler.revertModules()
			if err != nil {
				handler.switchState(cancelState, err)
				break
			}

			if rebootRequired {
				break
			}

			handler.switchState(finishState, nil)

		case cancelState:
			rebootRequired, err = handler.cancelRevertModules()
			if err != nil {
				log.Errorf("Error canceling revert modules: %s", err)
			}

			if rebootRequired {
				break
			}

			handler.finishOperation(handler.state.LastError)

			return

		case finishState:
			var err error

			if err = handler.finishRevertModules(); err != nil {
				log.Errorf("Error finishing revert modules: %s", err)
			}

			handler.finishOperation(err)

			return
		}

		if rebootRequired {
			log.Debug("System reboot is required")

			if err = handler.platform.SystemReboot(); err != nil {
				log.Errorf("Can't perform system reboot: %s", err)
			}

			return
		}
	}
}

func (handler *Handler) switchState(state operationState, lastError error) {
	handler.Lock()
	defer handler.Unlock()

	handler.state.OperationState = state
	handler.state.LastError = lastError

	log.WithFields(log.Fields{"state": handler.state.OperationState, "operation": handler.state.OperationType}).Debugf("State changed")

	if err := handler.setState(); err != nil {
		if handler.state.LastError != nil {
			handler.state.LastError = err
		}

		handler.finishOperation(handler.state.LastError)
	}
}

func (handler *Handler) unpackImage() (err error) {
	if err = os.RemoveAll(handler.bundleDir); err != nil {
		return err
	}

	if err = os.MkdirAll(handler.bundleDir, 0755); err != nil {
		return err
	}

	if err = image.UntarGZArchive(handler.state.ImagePath, handler.bundleDir); err != nil {
		return err
	}

	return nil
}

func (handler *Handler) getImageMetadata() (metadata imageMetadata, err error) {
	metadataJSON, err := ioutil.ReadFile(path.Join(handler.bundleDir, metadataFileName))
	if err != nil {
		return metadata, err
	}

	if err = json.Unmarshal(metadataJSON, &metadata); err != nil {
		return metadata, err
	}

	return metadata, err
}

func (handler *Handler) upgradeModules() (rebootRequired bool, err error) {
	metadata, err := handler.getImageMetadata()
	if err != nil {
		return false, err
	}

	platformID, err := handler.platform.GetPlatformID()
	if err != nil {
		return false, err
	}

	if metadata.PlatformID != platformID {
		return false, errors.New("wrong platform ID")
	}

	// Check that all modules were found
	for _, item := range metadata.UpdateItems {
		if _, ok := handler.modules[item.Type]; !ok {
			return false, fmt.Errorf("module %s not found", item.Type)
		}
	}

	for _, item := range metadata.UpdateItems {
		// Skip already upgraded modules
		if handler.state.ModuleStates[item.Type] == upgradedState {
			continue
		}

		if err = handler.setModuleState(item.Type, upgradingState); err != nil {
			return false, err
		}

		moduleRebootRequired, err := handler.modules[item.Type].Upgrade(handler.state.OperationVersion,
			path.Join(handler.bundleDir, item.Path))
		if err != nil {
			log.WithField("id", item.Type).Errorf("Upgrade module failed: %s", err)

			return false, err
		}

		if moduleRebootRequired {
			log.WithField("id", item.Type).Debug("Module reboot required")

			rebootRequired = true

			continue
		}

		if err = handler.setModuleState(item.Type, upgradedState); err != nil {
			return false, err
		}
	}

	return rebootRequired, nil
}

func (handler *Handler) cancelUpgradeModules() (rebootRequired bool, cancelErr error) {
	for id, state := range handler.state.ModuleStates {
		// Skip already canceled modules
		if state == canceledState {
			continue
		}

		if err := handler.setModuleState(id, cancelingState); err != nil {
			log.Errorf("Can't set module state: %s", err)

			if cancelErr == nil {
				cancelErr = err
			}
		}

		module, ok := handler.modules[id]
		if !ok {
			err := fmt.Errorf("module %s not found", id)

			log.Errorf("Error finishing upgrade module: %s", err)

			if cancelErr == nil {
				cancelErr = err
			}
		} else {
			moduleRebootRequired, err := module.CancelUpgrade(handler.state.OperationVersion)
			if err != nil {
				log.Errorf("Error canceling upgrade module: %s", err)

				if cancelErr == nil {
					cancelErr = err
				}
			}

			if moduleRebootRequired {
				log.WithField("id", id).Debug("Module reboot required")

				rebootRequired = true

				continue
			}
		}

		if err := handler.setModuleState(id, canceledState); err != nil {
			log.Errorf("Can't set module state: %s", err)

			if cancelErr == nil {
				cancelErr = err
			}
		}
	}

	return rebootRequired, cancelErr
}

func (handler *Handler) finishUpgradeModules() (finishErr error) {
	for id := range handler.state.ModuleStates {
		module, ok := handler.modules[id]
		if !ok {
			err := fmt.Errorf("module %s not found", id)

			log.Errorf("Error finishing upgrade module: %s", err)

			if finishErr == nil {
				finishErr = err
			}
		} else {
			if err := module.FinishUpgrade(handler.state.OperationVersion); err != nil {
				log.Errorf("Error finishing upgrade module: %s", err)

				if finishErr == nil {
					finishErr = err
				}
			}
		}
	}

	return finishErr
}

func (handler *Handler) revertModules() (rebootRequired bool, err error) {
	for id, state := range handler.state.ModuleStates {
		// Skip already reverted modules
		if state == revertedState {
			continue
		}

		module, ok := handler.modules[id]
		if !ok {
			return false, fmt.Errorf("module %s not found", id)
		}

		if err = handler.setModuleState(id, revertingState); err != nil {
			return false, err
		}

		moduleRebootRequired, err := module.Revert(handler.state.OperationVersion)
		if err != nil {
			log.WithField("id", id).Errorf("Revert module failed: %s", err)
			return false, err
		}

		if moduleRebootRequired {
			log.WithField("id", id).Debug("Module reboot required")

			rebootRequired = true

			continue
		}

		if err = handler.setModuleState(id, revertedState); err != nil {
			return false, err
		}
	}

	return rebootRequired, nil
}

func (handler *Handler) cancelRevertModules() (rebootRequired bool, cancelErr error) {
	for id, state := range handler.state.ModuleStates {
		// Skip already canceled modules
		if state == upgradedState || state == canceledState {
			continue
		}

		if err := handler.setModuleState(id, cancelingState); err != nil {
			log.Errorf("Can't set module state: %s", err)

			if cancelErr == nil {
				cancelErr = err
			}
		}

		module, ok := handler.modules[id]
		if !ok {
			err := fmt.Errorf("module %s not found", id)

			log.Errorf("Error finishing upgrade module: %s", err)

			if cancelErr == nil {
				cancelErr = err
			}
		} else {
			moduleRebootRequired, err := module.CancelRevert(handler.state.OperationVersion)

			if err != nil {
				log.Errorf("Error canceling revert module: %s", err)

				if cancelErr == nil {
					cancelErr = err
				}
			}

			if moduleRebootRequired {
				log.WithField("id", id).Debug("Module reboot required")

				rebootRequired = true
				continue
			}
		}

		if err := handler.setModuleState(id, canceledState); err != nil {
			log.Errorf("Can't set module state: %s", err)

			if cancelErr == nil {
				cancelErr = err
			}
		}
	}

	return rebootRequired, cancelErr
}

func (handler *Handler) finishRevertModules() (finishErr error) {
	for id := range handler.state.ModuleStates {
		module, ok := handler.modules[id]
		if !ok {
			err := fmt.Errorf("module %s not found", id)

			log.Errorf("Error finishing revert module: %s", err)

			if finishErr == nil {
				finishErr = err
			}
		} else {
			if err := module.FinishUpgrade(handler.state.OperationVersion); err != nil {
				log.Errorf("Error finishing revert module: %s", err)

				if finishErr == nil {
					finishErr = err
				}
			}
		}
	}

	return finishErr
}

func (handler *Handler) setModuleState(id string, state moduleState) (err error) {
	handler.Lock()
	defer handler.Unlock()

	log.WithFields(log.Fields{"state": state, "id": id}).Debugf("Module state changed")

	handler.state.ModuleStates[id] = state

	if err := handler.setState(); err != nil {
		return err
	}

	return nil
}
