package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mhyllander/rpi_plug_controller/controller"
	"github.com/mhyllander/rpi_plug_controller/db"
	"github.com/mhyllander/rpi_plug_controller/logs"
	"github.com/mhyllander/rpi_plug_controller/server"
)

func run(dbFile, plugPort string) error {
	store, err := db.Open(dbFile)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	plugController := controller.New(store, plugPort)
	controllerDone := make(chan struct{})
	go func() {
		defer close(controllerDone)
		plugController.Run(ctx)
	}()

	err = server.New(store, plugController).ListenAndServe(ctx)

	// stop the controller, whether the server was shut down or failed
	stop()
	<-controllerDone
	return err
}

func main() {
	var vDb = flag.String("db", db.GetDBPath(), "SQLite database")
	var vLogLevel = flag.String("log-level", "info", "log level [debug|info|warn|error]")
	var vPlugPort = flag.String("plug-port", "80", "port of the plugs' HTTP API")
	var vHelp = flag.Bool("help", false, "print usage")

	flag.Parse()

	if *vHelp {
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *vDb == "" {
		fmt.Fprintln(os.Stderr, "please set the db name")
		os.Exit(1)
	}

	if err := logs.InitLogger(*vLogLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := run(*vDb, *vPlugPort); err != nil {
		slog.Error("plugs controller failed", "err", err)
		os.Exit(1)
	}
}
