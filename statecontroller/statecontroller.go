package statecontroller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	log "github.com/sirupsen/logrus"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

const rootFSModuleID = "rootfs"
const bootloaderModuleID = "bootloader"

const (
	kernelRootPrefix = "root="
)

/*******************************************************************************
 * Types
 ******************************************************************************/

// Controller state controller instance
type Controller struct {
	moduleProvider ModuleProvider
	config         controllerConfig
	activeRootPart string
}

// ModuleProvider module provider interface
type ModuleProvider interface {
	// GetModuleByID returns module by id
	GetModuleByID(id string) (module interface{}, err error)
}

type partitionInfo struct {
	Device string
	FSType string
}

type controllerConfig struct {
	KernelCmdline  string
	RootPartitions []partitionInfo
}

type fsModule interface {
	SetPartitionForUpdate(path, fsType string) (err error)
}

/*******************************************************************************
 * Vars
 ******************************************************************************/

/*******************************************************************************
 * Public
 ******************************************************************************/

// New creates new state controller instance
func New(configJSON []byte, moduleProvider ModuleProvider) (controller *Controller, err error) {
	log.Info("Create state constoller")

	if moduleProvider == nil {
		return nil, errors.New("module provider should not be nil")
	}

	controller = &Controller{
		moduleProvider: moduleProvider,
		config: controllerConfig{
			KernelCmdline: "/proc/cmdline",
		},
	}

	if err = json.Unmarshal(configJSON, &controller.config); err != nil {
		return nil, err
	}

	if err = controller.parseBootCmd(); err != nil {
		return nil, err
	}

	if err = controller.initModules(); err != nil {
		return nil, err
	}

	return controller, nil
}

// Close closes state controller instance
func (controller *Controller) Close() (err error) {
	log.Info("Close state constoller")

	return nil
}

// GetVersion returns current installed image version
func (controller *Controller) GetVersion() (version uint64, err error) {
	return 0, nil
}

// GetPlatformID returns platform ID
func (controller *Controller) GetPlatformID() (id string, err error) {
	return "Nuance-OTA", nil
}

// Upgrade notifies state controller about start of system upgrade
func (controller *Controller) Upgrade(version uint64) (err error) {
	return nil
}

// Revert notifies state controller about start of system revert
func (controller *Controller) Revert(version uint64) (err error) {
	return nil
}

// UpgradeFinished notifies state controller about finish of upgrade
func (controller *Controller) UpgradeFinished(version uint64, status error, moduleStatus map[string]error) (postpone bool, err error) {
	return false, nil
}

// RevertFinished notifies state controller about finish of revert
func (controller *Controller) RevertFinished(version uint64, status error, moduleStatus map[string]error) (postpone bool, err error) {
	return false, nil
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func (controller *Controller) getRootFSUpdatePartition() (partition partitionInfo, err error) {
	for _, partition = range controller.config.RootPartitions {
		if partition.Device != controller.activeRootPart {
			log.WithField("partition", partition.Device).Debug("Update root partition")

			return partition, nil
		}
	}

	return partition, errors.New("no root FS update partition found")
}

func (controller *Controller) getBootloaderUpdatePartition() (partition partitionInfo, err error) {
	return partition, nil
}

func (controller *Controller) initModules() (err error) {
	if err := controller.initFileSystemUpdateModule(rootFSModuleID, controller.getRootFSUpdatePartition); err != nil {
		return err
	}

	if err := controller.initFileSystemUpdateModule(bootloaderModuleID, controller.getBootloaderUpdatePartition); err != nil {
		return err
	}

	return nil
}

func (controller *Controller) initFileSystemUpdateModule(id string, resourceProvider func() (partitionInfo, error)) (err error) {
	log.Info("Register module: ", id)

	module, err := controller.moduleProvider.GetModuleByID(id)
	if err != nil {
		return err
	}

	fsModule, ok := module.(fsModule)
	if !ok {
		return fmt.Errorf("module %s doesn't implement required interface", id)
	}

	partition, err := resourceProvider()
	if err != nil {
		return err
	}

	if err = fsModule.SetPartitionForUpdate(partition.Device, partition.FSType); err != nil {
		return err
	}

	return nil
}

func (controller *Controller) parseBootCmd() (err error) {
	data, err := ioutil.ReadFile(controller.config.KernelCmdline)
	if err != nil {
		return err
	}

	options := strings.Split(string(data), " ")

	for _, option := range options {
		option = strings.TrimSpace(option)

		switch {
		case strings.HasPrefix(option, kernelRootPrefix):
			controller.activeRootPart = strings.TrimPrefix(option, kernelRootPrefix)
		}
	}

	if controller.activeRootPart == "" {
		return errors.New("can't define active root FS")
	}

	log.WithField("partition", controller.activeRootPart).Debug("Active root partition")

	return nil
}
