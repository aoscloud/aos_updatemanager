package updatehandler

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"path"
	"strconv"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"gitpct.epam.com/epmd-aepr/aos_servicemanager/image"
	"gitpct.epam.com/epmd-aepr/aos_updatemanager/config"
	"gitpct.epam.com/epmd-aepr/aos_updatemanager/umserver"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

/*******************************************************************************
 * Types
 ******************************************************************************/

// Handler update handler
type Handler struct {
	sync.Mutex
	modules map[string]Module
	storage Storage

	versionFile      string
	upgradeDir       string
	state            int
	filesInfo        []umserver.UpgradeFileInfo
	imageVersion     uint64
	operationVersion uint64
	lastError        error
}

// Storage provides API to store/retreive persistent data
type Storage interface {
	SetState(state int) (err error)
	GetState() (state int, err error)
	SetFilesInfo(filesInfo []umserver.UpgradeFileInfo) (err error)
	GetFilesInfo() (filesInfo []umserver.UpgradeFileInfo, err error)
	SetOperationVersion(version uint64) (err error)
	GetOperationVersion() (version uint64, err error)
	SetLastError(lastError error) (err error)
	GetLastError() (lastError error, err error)
}

/*******************************************************************************
 * Public
 ******************************************************************************/

// New returns pointer to new Handler
func New(cfg *config.Config, storage Storage) (handler *Handler, err error) {
	log.Debug("Create update handler")

	handler = &Handler{
		storage:     storage,
		upgradeDir:  cfg.UpgradeDir,
		versionFile: cfg.VersionFile,
		modules:     make(map[string]Module)}

	if handler.imageVersion, err = handler.getImageVersion(); err != nil {
		return nil, err
	}

	if handler.state, err = handler.storage.GetState(); err != nil {
		return nil, err
	}

	if handler.filesInfo, err = handler.storage.GetFilesInfo(); err != nil {
		return nil, err
	}

	if handler.operationVersion, err = handler.storage.GetOperationVersion(); err != nil {
		return nil, err
	}

	if handler.lastError, err = handler.storage.GetLastError(); err != nil {
		return nil, err
	}

	for _, moduleCfg := range cfg.Modules {
		if moduleCfg.Disabled {
			log.WithField("id", moduleCfg.ID).Debug("Skip disabled module")
			continue
		}

		if _, ok := handler.modules[moduleCfg.ID]; ok {
			log.WithField("id", moduleCfg.ID).Warning("Module already exists")
		}

		paramsJSON, err := json.Marshal(moduleCfg.Params)
		if err != nil {
			return nil, err
		}

		module, err := newModule(moduleCfg.ID, moduleCfg.Plugin, paramsJSON)
		if err != nil {
			return nil, err
		}

		handler.modules[moduleCfg.ID] = module
	}

	if len(handler.modules) == 0 {
		return nil, errors.New("no valid modules info provided")
	}

	return handler, nil
}

// GetVersion returns current system version
func (handler *Handler) GetVersion() (version uint64) {
	handler.Lock()
	defer handler.Unlock()

	return handler.imageVersion
}

// GetOperationVersion returns upgrade/revert version
func (handler *Handler) GetOperationVersion() (version uint64) {
	handler.Lock()
	defer handler.Unlock()

	return handler.operationVersion
}

// GetState returns update state
func (handler *Handler) GetState() (state int) {
	handler.Lock()
	defer handler.Unlock()

	return handler.state
}

// GetLastError returns last upgrade error
func (handler *Handler) GetLastError() (err error) {
	handler.Lock()
	defer handler.Unlock()

	return handler.lastError
}

// Upgrade performs upgrade operation
func (handler *Handler) Upgrade(version uint64, filesInfo []umserver.UpgradeFileInfo) (err error) {
	handler.Lock()
	defer handler.Unlock()

	log.WithField("version", version).Info("Upgrade")

	if handler.state != umserver.RevertedState && handler.state != umserver.UpgradedState {
		return errors.New("wrong state")
	}

	if handler.state == umserver.RevertedState && handler.lastError != nil {
		return errors.New("can't upgrade after failed revert")
	}

	/* TODO: Shall image version be without gaps?
	if handler.imageVersion+1 != version {
		return errors.New("wrong version")
	}
	*/

	handler.state = umserver.UpgradingState

	if err = handler.storage.SetState(handler.state); err != nil {
		return err
	}

	handler.operationVersion = version

	if err = handler.storage.SetOperationVersion(handler.operationVersion); err != nil {
		return err
	}

	handler.filesInfo = filesInfo

	if err = handler.storage.SetFilesInfo(handler.filesInfo); err != nil {
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

	handler.state = umserver.UpgradedState

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

	if !(handler.state == umserver.UpgradedState && handler.lastError == nil) {
		return errors.New("wrong state")
	}

	if len(handler.filesInfo) == 0 {
		return errors.New("wrong state")
	}

	/* TODO: Shall image version be without gaps?
	if handler.imageVersion-1 != version {
		return errors.New("wrong version")
	}
	*/

	handler.state = umserver.RevertingState

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

	handler.filesInfo = nil

	if err = handler.storage.SetFilesInfo(handler.filesInfo); err != nil {
		return err
	}

	if err = handler.storage.SetLastError(handler.lastError); err != nil {
		return err
	}

	handler.state = umserver.RevertedState

	if err = handler.storage.SetState(handler.state); err != nil {
		return err
	}

	return handler.lastError
}

// Close closes update handler
func (handler *Handler) Close() {
	handler.Lock()
	defer handler.Unlock()

	for _, module := range handler.modules {
		module.Close()
	}
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (handler *Handler) getImageVersion() (version uint64, err error) {
	versionStr, err := ioutil.ReadFile(handler.versionFile)
	if err != nil {
		return 0, err
	}

	return strconv.ParseUint(strings.TrimSpace(string(versionStr)), 10, 64)
}

func (handler *Handler) setImageVersion(version uint64) (err error) {
	if err = ioutil.WriteFile(handler.versionFile, []byte(strconv.FormatUint(version, 10)), 0644); err != nil {
		return err
	}

	return nil
}

func (handler *Handler) upgrade() (err error) {
	// Called under locked context but we need to unlock here
	handler.Unlock()
	defer handler.Lock()

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

		module, ok := handler.modules[fileInfo.Target]
		if !ok {
			err = errors.New("missing component: " + fileInfo.Target)
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
			if err := handler.modules[handler.filesInfo[i].Target].Revert(); err != nil {
				return err
			}
		}

		return err
	}

	handler.imageVersion = handler.operationVersion

	if err = handler.setImageVersion(handler.imageVersion); err != nil {
		return err
	}

	return nil
}

func (handler *Handler) revert() (err error) {
	// Called under locked context but we need to unlock here
	handler.Unlock()
	defer handler.Lock()

	for _, fileInfo := range handler.filesInfo {
		module, ok := handler.modules[fileInfo.Target]
		if !ok {
			return errors.New("missing component: " + fileInfo.Target)
		}

		if err = module.Revert(); err != nil {
			return err
		}
	}

	handler.imageVersion = handler.operationVersion

	if err = handler.setImageVersion(handler.imageVersion); err != nil {
		return err
	}

	return nil
}
