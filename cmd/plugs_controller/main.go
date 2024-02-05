package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/mhyllander/rpi_plug_controller/db"
	"github.com/mhyllander/rpi_plug_controller/server"
	"github.com/mhyllander/rpi_plug_controller/utils"
)

func processPlugUpdates(plugUpdates <-chan string, done <-chan struct{}) {
	for {
		select {
		case plugId := <-plugUpdates:
			slog.Debug("processing state update for plug", "plugId", plugId)
			sensors, err := db.GetStatesForPlug(plugId)
			if err != nil {
				slog.Error("failed ot get states for plug", "plugId", plugId, "err", err)
				break
			}
			if len(sensors) == 0 {
				slog.Info("no sensors for plug", "plugId", plugId)
				break
			}
			// Check if any sensor wants the plug to be on
			state := "off"
			for _, s := range sensors {
				if s.State == "on" {
					state = "on"
					break
				}
			}
			// get the plug's IP address
			plug, err := db.GetPlug(plugId)
			if err != nil {
				slog.Error("failed to get plug", "plugId", plugId, "err", err)
				break
			}
			slog.Info("notify plug", "plugId", plug.PlugId, "state", state, "ipAddress", plug.IpAddress)
			resp, err := http.Get(fmt.Sprintf("http://%s/relay/0?turn=%s", plug.IpAddress, state))
			if err != nil {
				slog.Warn("failed to notify plug", "plugId", plug.PlugId, "err", err)
			}
			slog.Debug("notify plug response", "resp", resp)

		case <-done:
			return
		}
	}
}

func main() {
	var vDb = flag.String("db", db.GetDBPath(), "SQLite database")
	var vLogLevel = flag.String("log-level", "info", "log level [debug|info|warn|error]")
	var vHelp = flag.Bool("help", false, "print usage")

	flag.Parse()

	if *vHelp {
		flag.PrintDefaults()
		os.Exit(0)
	}

	if *vDb == "" {
		fmt.Printf("please set the db name")
		os.Exit(1)
	}

	utils.InitLogger(*vLogLevel)

	// open and initialize database
	db.Initialize(*vDb)
	defer db.Close()

	done := make(chan struct{})
	defer close(done)

	plugUpdates := make(chan string)
	defer close(plugUpdates)

	go processPlugUpdates(plugUpdates, done)

	server.StartServer(plugUpdates, *vLogLevel)
}
