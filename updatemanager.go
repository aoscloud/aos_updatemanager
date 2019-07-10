package main

import (
	"flag"
	"os"
	"os/signal"
	"path"
	"syscall"

	"gitpct.epam.com/epmd-aepr/aos_updatemanager/updatehandler"

	log "github.com/sirupsen/logrus"

	"gitpct.epam.com/epmd-aepr/aos_updatemanager/config"
	"gitpct.epam.com/epmd-aepr/aos_updatemanager/database"
	"gitpct.epam.com/epmd-aepr/aos_updatemanager/umserver"
)

/*******************************************************************************
 * Consts
 ******************************************************************************/

const dbFileName = "updatemanager.db"

/*******************************************************************************
 * Vars
 ******************************************************************************/

// GitSummary provided by govvv at compile-time
var GitSummary string

/*******************************************************************************
 * Init
 ******************************************************************************/

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: false,
		TimestampFormat:  "2006-01-02 15:04:05.000",
		FullTimestamp:    true})
	log.SetOutput(os.Stdout)
}

/*******************************************************************************
 * Private
 ******************************************************************************/

func cleanup(workingDir, dbFile string) {
	log.Debug("System cleanup")

	log.WithField("file", dbFile).Debug("Delete DB file")
	if err := os.RemoveAll(dbFile); err != nil {
		log.Fatalf("Can't cleanup database: %s", err)
	}
}

/*******************************************************************************
 * Main
 ******************************************************************************/

func main() {
	// Initialize command line flags
	configFile := flag.String("c", "aos_updatemanager.cfg", "path to config file")
	strLogLevel := flag.String("v", "info", `log level: "debug", "info", "warn", "error", "fatal", "panic"`)

	flag.Parse()

	// Set log level
	logLevel, err := log.ParseLevel(*strLogLevel)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
	log.SetLevel(logLevel)

	log.WithFields(log.Fields{"configFile": *configFile, "version": GitSummary}).Info("Start update manager")

	cfg, err := config.New(*configFile)
	if err != nil {
		log.Fatalf("Can' open config file: %s", err)
	}

	// Create DB
	dbFile := path.Join(cfg.WorkingDir, dbFileName)

	db, err := database.New(dbFile)
	if err != nil {
		if err == database.ErrVersionMismatch {
			log.Warning("Unsupported database version")
			cleanup(cfg.WorkingDir, dbFile)
			db, err = database.New(dbFile)
		}

		if err != nil {
			log.Fatalf("Can't create database: %s", err)
		}
	}

	updater, err := updatehandler.New(cfg, db)
	if err != nil {
		log.Fatalf("Can't create updater: %s", err)
	}
	defer updater.Close()

	server, err := umserver.New(cfg, updater)
	if err != nil {
		log.Fatalf("Can't create UM server: %s", err)
	}
	defer server.Close()

	// Handle SIGTERM
	terminateChannel := make(chan os.Signal, 1)
	signal.Notify(terminateChannel, os.Interrupt, syscall.SIGTERM)

	<-terminateChannel
}
