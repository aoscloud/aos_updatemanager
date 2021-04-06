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
	"net/url"
	"os"
	"sort"
	"sync"

	"github.com/cavaliercoder/grab"
	"github.com/looplab/fsm"
	log "github.com/sirupsen/logrus"
	"gitpct.epam.com/epmd-aepr/aos_common/image"

	"aos_updatemanager/config"
	"aos_updatemanager/umclient"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

const statusChannelSize = 1

const (
	eventPrepare = "prepare"
	eventUpdate  = "update"
	eventApply   = "apply"
	eventRevert  = "revert"
)

const (
	stateIdle     = "idle"
	statePrepared = "prepared"
	stateUpdated  = "updated"
	stateFailed   = "failed"
)

/*******************************************************************************
 * Vars
 ******************************************************************************/

var plugins = make(map[string]NewPlugin)

/*******************************************************************************
 * Types
 ******************************************************************************/

// Handler update handler
type Handler struct {
	sync.Mutex

	storage           StateStorage
	components        map[string]componentData
	componentStatuses map[string]*umclient.ComponentStatusInfo
	state             handlerState
	initWG            sync.WaitGroup
	fsm               *fsm.FSM
	downloadDir       string

	statusChannel chan umclient.Status
}

// UpdateModule interface for module plugin
type UpdateModule interface {
	// GetID returns module ID
	GetID() (id string)
	// GetVendorVersion returns vendor version
	GetVendorVersion() (version string, err error)
	// Init initializes module
	Init() (err error)
	// Prepare prepares module
	Prepare(imagePath string, vendorVersion string, annotations json.RawMessage) (err error)
	// Update updates module
	Update() (rebootRequired bool, err error)
	// Apply applies update
	Apply() (rebootRequired bool, err error)
	// Revert reverts update
	Revert() (rebootRequired bool, err error)
	// Reboot performs module reboot
	Reboot() (err error)
	// Close closes update module
	Close() (err error)
}

// StateStorage provides API to store/retreive persistent data
type StateStorage interface {
	SetUpdateState(state []byte) (err error)
	GetUpdateState() (state []byte, err error)
	SetAosVersion(id string, version uint64) (err error)
	GetAosVersion(id string) (version uint64, err error)
	SetVendorVersion(id string, version string) (err error)
	GetVendorVersion(id string) (version string, err error)
}

// ModuleStorage provides API store/retrive module persistent data
type ModuleStorage interface {
	SetModuleState(id string, state []byte) (err error)
	GetModuleState(id string) (state []byte, err error)
}

// NewPlugin update module new function
type NewPlugin func(id string, configJSON json.RawMessage, storage ModuleStorage) (module UpdateModule, err error)

type handlerState struct {
	UpdateState       string                                   `json:"updateState"`
	Error             string                                   `json:"error"`
	ComponentStatuses map[string]*umclient.ComponentStatusInfo `json:"componentStatuses"`
}

type componentData struct {
	module         UpdateModule
	updatePriority uint32
	rebootPriority uint32
}

type componentOperation func(module UpdateModule) (rebootRequired bool, err error)

type priorityOperation struct {
	priority  uint32
	operation func() (err error)
}

/*******************************************************************************
 * Public
 ******************************************************************************/

// RegisterPlugin registers update plugin
func RegisterPlugin(plugin string, newFunc NewPlugin) {
	log.WithField("plugin", plugin).Info("Register update plugin")

	plugins[plugin] = newFunc
}

// New returns pointer to new Handler
func New(cfg *config.Config, storage StateStorage, moduleStorage ModuleStorage) (handler *Handler, err error) {
	log.Debug("Create update handler")

	handler = &Handler{
		componentStatuses: make(map[string]*umclient.ComponentStatusInfo),
		storage:           storage,
		statusChannel:     make(chan umclient.Status, statusChannelSize),
		downloadDir:       cfg.DownloadDir,
	}

	if err = handler.getState(); err != nil {
		return nil, err
	}

	if handler.state.UpdateState == "" {
		handler.state.UpdateState = stateIdle
	}

	handler.fsm = fsm.NewFSM(handler.state.UpdateState, fsm.Events{
		{Name: eventPrepare, Src: []string{stateIdle}, Dst: statePrepared},
		{Name: eventUpdate, Src: []string{statePrepared}, Dst: stateUpdated},
		{Name: eventApply, Src: []string{stateUpdated}, Dst: stateIdle},
		{Name: eventRevert, Src: []string{statePrepared, stateUpdated, stateFailed}, Dst: stateIdle},
	},
		fsm.Callbacks{
			"after_event":           handler.onStateChanged,
			"leave_state":           func(e *fsm.Event) { e.Async() },
			"after_" + eventPrepare: handler.onPrepareState,
			"after_" + eventUpdate:  handler.onUpdateState,
			"after_" + eventApply:   handler.onApplyState,
			"after_" + eventRevert:  handler.onRevertState,
		},
	)

	handler.components = make(map[string]componentData)

	for _, moduleCfg := range cfg.UpdateModules {
		if moduleCfg.Disabled {
			log.WithField("id", moduleCfg.ID).Debug("Skip disabled module")
			continue
		}

		component := componentData{updatePriority: moduleCfg.UpdatePriority, rebootPriority: moduleCfg.RebootPriority}

		if component.module, err = handler.createComponent(moduleCfg.Plugin, moduleCfg.ID,
			moduleCfg.Params, moduleStorage); err != nil {
			return nil, err
		}

		handler.components[moduleCfg.ID] = component
	}

	handler.initWG.Add(1)
	go handler.init()

	return handler, nil
}

// Registered indicates the client registed to the server
func (handler *Handler) Registered() {
	go func() {
		// Wait for all modules are initialized before sending status
		handler.initWG.Wait()

		handler.sendStatus()
	}()

}

// PrepareUpdate prepares update
func (handler *Handler) PrepareUpdate(components []umclient.ComponentUpdateInfo) {
	log.Info("Prepare update")

	if err := handler.sendEvent(eventPrepare, components); err != nil {
		log.Errorf("Can't send prepare event: %s", err)
	}
}

// StartUpdate starts update
func (handler *Handler) StartUpdate() {
	log.Info("Start update")

	if err := handler.sendEvent(eventUpdate); err != nil {
		log.Errorf("Can't send update event: %s", err)
	}
}

// ApplyUpdate applies update
func (handler *Handler) ApplyUpdate() {
	log.Info("Apply update")

	if err := handler.sendEvent(eventApply); err != nil {
		log.Errorf("Can't send apply event: %s", err)
	}
}

// RevertUpdate reverts update
func (handler *Handler) RevertUpdate() {
	log.Info("Revert update")

	if err := handler.sendEvent(eventRevert); err != nil {
		log.Errorf("Can't send revert event: %s", err)
	}
}

// StatusChannel returns status channel
func (handler *Handler) StatusChannel() (status <-chan umclient.Status) {
	return handler.statusChannel
}

// Close closes update handler
func (handler *Handler) Close() {
	log.Debug("Close update handler")

	for _, component := range handler.components {
		component.module.Close()
	}
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (handler *Handler) createComponent(plugin, id string, params json.RawMessage, storage ModuleStorage) (module UpdateModule, err error) {
	newFunc, ok := plugins[plugin]
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", plugin)
	}

	if module, err = newFunc(id, params, storage); err != nil {
		return nil, err
	}

	return module, nil
}

func (handler *Handler) getState() (err error) {
	jsonState, err := handler.storage.GetUpdateState()
	if err != nil {
		return err
	}

	if len(jsonState) == 0 {
		return nil
	}

	if err = json.Unmarshal(jsonState, &handler.state); err != nil {
		return err
	}

	return nil
}

func (handler *Handler) saveState() (err error) {
	jsonState, err := json.Marshal(handler.state)
	if err != nil {
		return err
	}

	if err = handler.storage.SetUpdateState(jsonState); err != nil {
		return err
	}

	return nil
}

func (handler *Handler) init() {
	defer handler.initWG.Done()

	var operations []priorityOperation

	for id, component := range handler.components {
		handler.componentStatuses[id] = &umclient.ComponentStatusInfo{
			ID:     id,
			Status: umclient.StatusInstalled,
		}

		module := component.module
		id := id

		operations = append(operations, priorityOperation{
			priority: component.updatePriority,
			operation: func() (err error) {
				log.WithField("id", id).Debug("Init component")

				if err := module.Init(); err != nil {
					log.Errorf("Can't initialize module %s: %s", id, err)

					handler.componentStatuses[id].Status = umclient.StatusError
					handler.componentStatuses[id].Error = err.Error()
				}

				return nil
			},
		})
	}

	doPriorityOperations(operations, false)

	handler.getVersions()
}

func (handler *Handler) getVersions() {
	log.Debug("Update component versions")

	for id, component := range handler.components {
		var vendorVersion string
		var err error

		if handler.state.UpdateState == stateIdle {
			vendorVersion, err = component.module.GetVendorVersion()
			if err != nil {
				log.Errorf("Can't get vendor version from module: %s", err)
			}
		} else {
			vendorVersion, err = handler.storage.GetVendorVersion(id)
			if err != nil {
				log.Errorf("Can't get vendor version from storage: %s", err)
			}
		}

		handler.componentStatuses[id].VendorVersion = vendorVersion

		aosVersion, err := handler.storage.GetAosVersion(id)
		if err == nil {
			handler.componentStatuses[id].AosVersion = aosVersion
		}
	}
}

func (handler *Handler) sendStatus() {
	log.WithFields(log.Fields{"state": handler.state.UpdateState, "error": handler.state.Error}).Debug("Send status")

	status := umclient.Status{
		State: toUMState(handler.state.UpdateState),
		Error: handler.state.Error,
	}

	for id, componentStatus := range handler.componentStatuses {
		status.Components = append(status.Components, *componentStatus)

		log.WithFields(log.Fields{
			"id":            componentStatus.ID,
			"vendorVersion": componentStatus.VendorVersion,
			"aosVersion":    componentStatus.AosVersion,
			"status":        componentStatus.Status,
			"error":         componentStatus.Error}).Debug("Component status")

		updateStatus, ok := handler.state.ComponentStatuses[id]
		if ok {
			status.Components = append(status.Components, *updateStatus)

			log.WithFields(log.Fields{
				"id":            updateStatus.ID,
				"vendorVersion": updateStatus.VendorVersion,
				"aosVersion":    updateStatus.AosVersion,
				"status":        updateStatus.Status,
				"error":         updateStatus.Error}).Debug("Component status")
		}
	}

	handler.statusChannel <- status
}

func (handler *Handler) onStateChanged(event *fsm.Event) {
	handler.state.UpdateState = handler.fsm.Current()

	if handler.state.UpdateState == stateIdle {
		handler.getVersions()

		for id, componentStatus := range handler.state.ComponentStatuses {
			if componentStatus.Status != umclient.StatusError {
				delete(handler.state.ComponentStatuses, id)
			}
		}

		if handler.downloadDir != "" {
			if err := os.RemoveAll(handler.downloadDir); err != nil {
				log.Errorf("Can't remove download dir: %s", handler.downloadDir)
			}

			if err := os.MkdirAll(handler.downloadDir, 0755); err != nil {
				log.Errorf("Can't create download dir: %s", handler.downloadDir)
			}
		}
	}

	if err := handler.saveState(); err != nil {
		log.Errorf("Can't set update state: %s", err)

		if handler.state.Error == "" {
			handler.state.Error = err.Error()
		}

		handler.state.UpdateState = stateFailed
		handler.fsm.SetState(handler.state.UpdateState)
	}

	handler.sendStatus()
}

func componentError(componentStatus *umclient.ComponentStatusInfo, err error) {
	log.WithField("id", componentStatus.ID).Errorf("Component error: %s", err)

	componentStatus.Status = umclient.StatusError
	componentStatus.Error = err.Error()
}

func doPriorityOperations(operations []priorityOperation, stopOnError bool) (err error) {
	if len(operations) == 0 {
		return nil
	}

	sort.Slice(operations, func(i, j int) bool { return operations[i].priority > operations[j].priority })

	var wg sync.WaitGroup
	var groupErr error
	priority := operations[0].priority

	for _, item := range operations {
		if item.priority != priority {
			wg.Wait()

			if groupErr != nil {
				if stopOnError {
					return groupErr
				}

				if err == nil {
					err = groupErr
				}

				groupErr = nil
			}

			priority = item.priority
		}

		operation := item.operation

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := operation(); err != nil {
				if groupErr == nil {
					groupErr = err
				}
			}
		}()
	}

	wg.Wait()

	if groupErr != nil {
		if err == nil {
			err = groupErr
		}
	}

	return err
}

func (handler *Handler) doOperation(componentStatuses []*umclient.ComponentStatusInfo,
	operation componentOperation, stopOnError bool) (rebootStatuses []*umclient.ComponentStatusInfo, err error) {
	var operations []priorityOperation

	for _, componentStatus := range componentStatuses {
		component, ok := handler.components[componentStatus.ID]
		if !ok {
			notFoundErr := fmt.Errorf("component %s not found", componentStatus.ID)
			componentError(componentStatus, notFoundErr)

			if stopOnError {
				return nil, notFoundErr
			}

			if err == nil {
				err = notFoundErr
			}

			continue
		}

		module := component.module
		status := componentStatus

		operations = append(operations, priorityOperation{
			priority: component.updatePriority,
			operation: func() (err error) {
				rebootRequired, err := operation(module)
				if err != nil {
					componentError(status, err)
					return err
				}

				if rebootRequired {
					log.WithField("id", module.GetID()).Debug("Reboot required")

					rebootStatuses = append(rebootStatuses, status)
				}

				return nil
			},
		})
	}

	err = doPriorityOperations(operations, stopOnError)

	return rebootStatuses, err
}

func (handler *Handler) doReboot(componentStatuses []*umclient.ComponentStatusInfo, stopOnError bool) (err error) {
	var operations []priorityOperation

	for _, componentStatus := range componentStatuses {
		component, ok := handler.components[componentStatus.ID]
		if !ok {
			notFoundErr := fmt.Errorf("component %s not found", componentStatus.ID)
			componentError(componentStatus, notFoundErr)

			if stopOnError {
				return notFoundErr
			}

			if err == nil {
				err = notFoundErr
			}

			continue
		}

		module := component.module

		operations = append(operations, priorityOperation{
			priority: component.rebootPriority,
			operation: func() (err error) {
				log.WithField("id", module.GetID()).Debug("Reboot component")

				if err := module.Reboot(); err != nil {
					componentError(componentStatus, err)
					return err
				}

				return nil
			},
		})
	}

	return doPriorityOperations(operations, stopOnError)
}

func (handler *Handler) componentOperation(operation componentOperation, stopOnError bool) (err error) {
	var operationStatuses []*umclient.ComponentStatusInfo

	for _, operationStatus := range handler.state.ComponentStatuses {
		operationStatuses = append(operationStatuses, operationStatus)
	}

	for len(operationStatuses) != 0 {
		rebootStatuses, opError := handler.doOperation(operationStatuses, operation, stopOnError)
		if opError != nil {
			if stopOnError {
				return opError
			}

			if err == nil {
				err = opError
			}
		}

		if len(rebootStatuses) == 0 {
			return err
		}

		if rebootError := handler.doReboot(rebootStatuses, stopOnError); rebootError != nil {
			if stopOnError {
				return rebootError
			}

			if err == nil {
				err = rebootError
			}
		}

		operationStatuses = rebootStatuses
	}

	return err
}

func downloadImage(downloadDir, urlStr string) (filePath string, err error) {
	var urlVal *url.URL

	if urlVal, err = url.Parse(urlStr); err != nil {
		return "", err
	}

	if urlVal.Scheme == "file" {
		return urlVal.Path, nil
	}

	grabClient := grab.NewClient()

	log.WithField("url", urlStr).Debug("Start downloading file")

	req, err := grab.NewRequest(downloadDir, urlStr)
	if err != nil {
		return "", err
	}

	resp := grabClient.Do(req)

	<-resp.Done

	if err = resp.Err(); err != nil {
		return "", err
	}

	log.WithField("file", resp.Filename).Debug("Download complete")

	return resp.Filename, nil
}

func (handler *Handler) prepareComponent(module UpdateModule, updateInfo *umclient.ComponentUpdateInfo) (err error) {
	if err := handler.storage.SetVendorVersion(updateInfo.ID, handler.componentStatuses[updateInfo.ID].VendorVersion); err != nil {
		log.Errorf("Can't set vendor version: %s", err)

		return err
	}

	vendorVersion, err := module.GetVendorVersion()
	if err == nil && updateInfo.VendorVersion != "" {
		if vendorVersion == updateInfo.VendorVersion {
			return fmt.Errorf("Component already has required vendor version: %s", vendorVersion)
		}
	}

	if updateInfo.AosVersion != 0 {
		aosVersion, err := handler.storage.GetAosVersion(updateInfo.ID)

		if err == nil {
			if aosVersion == updateInfo.AosVersion {
				return fmt.Errorf("Component already has required Aos version: %d", updateInfo.AosVersion)
			}

			if aosVersion > updateInfo.AosVersion {
				return errors.New("wrong Aos version")
			}
		}
	}

	filePath, err := downloadImage(handler.downloadDir, updateInfo.URL)
	if err != nil {
		return err
	}

	if err = image.CheckFileInfo(filePath, image.FileInfo{
		Sha256: updateInfo.Sha256,
		Sha512: updateInfo.Sha512,
		Size:   updateInfo.Size}); err != nil {
		return err
	}

	if err = module.Prepare(filePath, updateInfo.VendorVersion, updateInfo.Annotations); err != nil {
		return err
	}

	return nil
}

func (handler *Handler) onPrepareState(event *fsm.Event) {
	componentsInfo := make(map[string]*umclient.ComponentUpdateInfo)

	handler.state.Error = ""
	handler.state.ComponentStatuses = make(map[string]*umclient.ComponentStatusInfo)

	infos := event.Args[0].([]umclient.ComponentUpdateInfo)

	if len(infos) == 0 {
		handler.state.Error = "Prepare componenet list is empty"
		handler.fsm.SetState(stateFailed)

		return
	}

	for i, info := range infos {
		componentsInfo[info.ID] = &infos[i]

		handler.state.ComponentStatuses[info.ID] = &umclient.ComponentStatusInfo{
			ID:            info.ID,
			VendorVersion: info.VendorVersion,
			AosVersion:    info.AosVersion,
			Status:        umclient.StatusInstalling,
		}
	}

	if err := handler.componentOperation(func(module UpdateModule) (rebootRequired bool, err error) {
		updateInfo, ok := componentsInfo[module.GetID()]
		if !ok {
			return false, fmt.Errorf("update info for %s component not found", module.GetID())
		}

		log.WithFields(log.Fields{
			"id":            updateInfo.ID,
			"vendorVersion": updateInfo.VendorVersion,
			"aosVersion":    updateInfo.AosVersion,
			"url":           updateInfo.URL}).Debug("Prepare component")

		return false, handler.prepareComponent(module, updateInfo)
	}, true); err != nil {
		handler.state.Error = err.Error()
		handler.fsm.SetState(stateFailed)
	}
}

func (handler *Handler) onUpdateState(event *fsm.Event) {
	handler.state.Error = ""

	if err := handler.componentOperation(func(module UpdateModule) (rebootRequired bool, err error) {
		log.WithFields(log.Fields{"id": module.GetID()}).Debug("Update component")

		rebootRequired, err = module.Update()
		if err != nil {
			return false, err
		}

		if !rebootRequired {
			vendorVersion, err := module.GetVendorVersion()
			if err != nil {
				return false, err
			}

			if vendorVersion != handler.state.ComponentStatuses[module.GetID()].VendorVersion {
				return false, fmt.Errorf("versions mismatch in request %s and updated module %s",
					handler.state.ComponentStatuses[module.GetID()].VendorVersion, vendorVersion)
			}
		}

		return rebootRequired, err
	}, true); err != nil {
		handler.state.Error = err.Error()
		handler.fsm.SetState(stateFailed)
	}
}

func (handler *Handler) onApplyState(event *fsm.Event) {
	handler.state.Error = ""

	if err := handler.componentOperation(func(module UpdateModule) (rebootRequired bool, err error) {
		log.WithFields(log.Fields{"id": module.GetID()}).Debug("Apply component")

		if rebootRequired, err = module.Apply(); err != nil {
			return rebootRequired, err
		}

		componentStatus, ok := handler.state.ComponentStatuses[module.GetID()]
		if !ok {
			return rebootRequired, fmt.Errorf("component %s status not found", module.GetID())
		}

		if err = handler.storage.SetAosVersion(componentStatus.ID, componentStatus.AosVersion); err != nil {
			return rebootRequired, err
		}

		return rebootRequired, nil
	}, false); err != nil {
		log.Errorf("Can't apply update: %s", err)
		handler.state.Error = err.Error()
	}
}

func (handler *Handler) onRevertState(event *fsm.Event) {
	handler.state.Error = ""

	if err := handler.componentOperation(func(module UpdateModule) (rebootRequired bool, err error) {
		log.WithFields(log.Fields{"id": module.GetID()}).Debug("Revert component")

		return module.Revert()
	}, false); err != nil {
		log.Errorf("Can't revert update: %s", err)
		handler.state.Error = err.Error()
	}
}

func (handler *Handler) sendEvent(event string, args ...interface{}) (err error) {
	if handler.fsm.Cannot(event) {
		return fmt.Errorf("error sending event %s in state: %s", event, handler.fsm.Current())
	}

	if err = handler.fsm.Event(event, args...); err != nil {
		if _, ok := err.(fsm.AsyncError); !ok {
			return err
		}

		go func() {
			if err := handler.fsm.Transition(); err != nil {
				log.Errorf("Error transition event %s: %s", event, err)
			}
		}()
	}

	return nil
}

func toUMState(state string) (umState umclient.UMState) {
	return map[string]umclient.UMState{
		stateIdle:     umclient.StateIdle,
		statePrepared: umclient.StatePrepared,
		stateUpdated:  umclient.StateUpdated,
		stateFailed:   umclient.StateFailed,
	}[state]
}
