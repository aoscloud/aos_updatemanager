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
	"errors"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
	"gitpct.epam.com/nunc-ota/aos_common/umprotocol"

	"aos_updatemanager/config"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

// State
const (
	UpgradedState = iota
	UpgradingState
	RevertedState
	RevertingState
)

/*******************************************************************************
 * Types
 ******************************************************************************/

// Handler update handler
type Handler struct {
	sync.Mutex
	storage        Storage
	moduleProvider ModuleProvider

	upgradeDir       string
	state            int
	currentVersion   uint64
	operationVersion uint64
	lastError        error
}

// UpdateModule interface for module plugin
type UpdateModule interface {
	// GetID returns module ID
	GetID() (id string)
	// Upgrade upgrade module
	Upgrade(fileName string) (err error)
	// Revert revert module
	Revert() (err error)
}

// ModuleProvider module provider interface
type ModuleProvider interface {
	// GetModuleByID returns module by id
	GetModuleByID(id string) (module interface{}, err error)
}

// Storage provides API to store/retreive persistent data
type Storage interface {
	SetState(state int) (err error)
	GetState() (state int, err error)
	SetCurrentVersion(version uint64) (err error)
	GetCurrentVersion() (version uint64, err error)
	SetOperationVersion(version uint64) (err error)
	GetOperationVersion() (version uint64, err error)
	SetLastError(lastError error) (err error)
	GetLastError() (lastError error, err error)
}

/*******************************************************************************
 * Public
 ******************************************************************************/

// New returns pointer to new Handler
func New(cfg *config.Config, moduleProvider ModuleProvider, storage Storage) (handler *Handler, err error) {
	log.Debug("Create update handler")

	handler = &Handler{
		storage:        storage,
		moduleProvider: moduleProvider,
		upgradeDir:     cfg.UpgradeDir,
	}

	if handler.currentVersion, err = storage.GetCurrentVersion(); err != nil {
		return nil, err
	}

	if handler.state, err = handler.storage.GetState(); err != nil {
		return nil, err
	}

	if handler.state == RevertingState || handler.state == UpgradingState {
		handler.lastError = errors.New("unknown error")

		log.Errorf("Last update failed: %s", handler.lastError)

		if err = handler.storage.SetLastError(handler.lastError); err != nil {
			return nil, err
		}

		switch {
		case handler.state == RevertingState:
			handler.state = RevertedState

		case handler.state == UpgradingState:
			handler.state = UpgradedState
		}

		if err = handler.storage.SetState(handler.state); err != nil {
			return nil, err
		}
	}

	if handler.operationVersion, err = handler.storage.GetOperationVersion(); err != nil {
		return nil, err
	}

	if handler.lastError, err = handler.storage.GetLastError(); err != nil {
		return nil, err
	}

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

	return handler.operationVersion
}

// GetStatus returns update status
func (handler *Handler) GetStatus() (status string) {
	handler.Lock()
	defer handler.Unlock()

	status = umprotocol.SuccessStatus

	if handler.state == RevertingState || handler.state == UpgradingState {
		status = umprotocol.InProgressStatus
	}

	if handler.lastError != nil {
		status = umprotocol.FailedStatus
	}

	return status
}

// GetLastOperation returns last operation
func (handler *Handler) GetLastOperation() (operation string) {
	handler.Lock()
	defer handler.Unlock()

	if handler.state == RevertingState || handler.state == RevertedState {
		operation = umprotocol.RevertOperation
	}

	if handler.state == UpgradingState || handler.state == UpgradedState {
		operation = umprotocol.UpgradeOperation
	}

	return operation
}

// GetLastError returns last upgrade error
func (handler *Handler) GetLastError() (err error) {
	handler.Lock()
	defer handler.Unlock()

	return handler.lastError
}

// Upgrade performs upgrade operation
func (handler *Handler) Upgrade(version uint64, imageInfo umprotocol.ImageInfo) (err error) {
	handler.Lock()
	defer handler.Unlock()

	log.WithField("version", version).Info("Upgrade")

	if handler.state != RevertedState && handler.state != UpgradedState {
		return errors.New("wrong state")
	}

	if handler.state == RevertedState && handler.lastError != nil {
		return errors.New("can't upgrade after failed revert")
	}

	/* TODO: Shall image version be without gaps?
	if handler.imageVersion+1 != version {
		return errors.New("wrong version")
	}
	*/

	handler.state = UpgradingState

	if err = handler.storage.SetState(handler.state); err != nil {
		return err
	}

	handler.operationVersion = version

	if err = handler.storage.SetOperationVersion(handler.operationVersion); err != nil {
		return err
	}

	handler.lastError = nil

	if err = handler.storage.SetLastError(handler.lastError); err != nil {
		return err
	}

	if err = handler.upgrade(); err != nil {
		handler.lastError = err
	}

	if err = handler.storage.SetLastError(handler.lastError); err != nil {
		return err
	}

	handler.state = UpgradedState

	if err = handler.storage.SetState(handler.state); err != nil {
		return err
	}

	return handler.lastError
}

// Revert performs revert operation
func (handler *Handler) Revert(version uint64) (err error) {
	handler.Lock()
	defer handler.Unlock()

	log.WithField("version", version).Info("Revert")

	if !(handler.state == UpgradedState && handler.lastError == nil) {
		return errors.New("wrong state")
	}

	/* TODO: Shall image version be without gaps?
	if handler.imageVersion-1 != version {
		return errors.New("wrong version")
	}
	*/

	handler.state = RevertingState

	if err = handler.storage.SetState(handler.state); err != nil {
		return err
	}

	handler.operationVersion = version

	if err = handler.storage.SetOperationVersion(handler.operationVersion); err != nil {
		return err
	}

	handler.lastError = nil

	if err = handler.storage.SetLastError(handler.lastError); err != nil {
		return err
	}

	if err = handler.revert(); err != nil {
		handler.lastError = err
	}

	if err = handler.storage.SetLastError(handler.lastError); err != nil {
		return err
	}

	handler.state = RevertedState

	if err = handler.storage.SetState(handler.state); err != nil {
		return err
	}

	return handler.lastError
}

// Close closes update handler
func (handler *Handler) Close() {
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (handler *Handler) upgrade() (err error) {
	// Called under locked context but we need to unlock here
	handler.Unlock()
	defer handler.Lock()

	// TODO: should be reimplemented a
	/*
		index := 0

		for ; index < len(handler.filesInfo); index++ {
			fileInfo := handler.filesInfo[index]
			fileName := path.Join(handler.upgradeDir, fileInfo.URL)

			if err = image.CheckFileInfo(fileName, image.FileInfo{
				Sha256: fileInfo.Sha256,
				Sha512: fileInfo.Sha512,
				Size:   fileInfo.Size}); err != nil {
				break
			}

			var module UpdateModule

			if module, err = handler.getModuleByID(fileInfo.Target); err != nil {
				break
			}

			if err = module.Upgrade(fileName); err != nil {
				// revert module with upgrade attempt
				index++
				break
			}
		}

		if err != nil {
			for i := 0; i < index; i++ {
				module, err := handler.getModuleByID(handler.filesInfo[i].Target)
				if err != nil {
					return err
				}

				if err = module.Revert(); err != nil {
					return err
				}
			}

			return err
		}

		handler.imageVersion = handler.operationVersion

		if err = handler.setImageVersion(handler.imageVersion); err != nil {
			return err
		}
	*/
	return nil
}

func (handler *Handler) revert() (err error) {
	// Called under locked context but we need to unlock here
	handler.Unlock()
	defer handler.Lock()

	// TODO: should be reimplemented according to new protocol
	/*
		for _, fileInfo := range handler.filesInfo {
			module, err := handler.getModuleByID(fileInfo.Target)
			if err != nil {
				return err
			}

			if err = module.Revert(); err != nil {
				return err
			}
		}

		handler.imageVersion = handler.operationVersion

		if err = handler.setImageVersion(handler.imageVersion); err != nil {
			return err
		}
	*/
	return nil
}

func (handler *Handler) getModuleByID(id string) (module UpdateModule, err error) {
	providedModule, err := handler.moduleProvider.GetModuleByID(id)
	if err != nil {
		return nil, err
	}

	updateModule, ok := providedModule.(UpdateModule)
	if !ok {
		return nil, fmt.Errorf("module %s doesn't provide required interface", id)
	}

	return updateModule, nil
}
