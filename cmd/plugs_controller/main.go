package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mhyllander/rpi_plug_controller/db"
	"github.com/mhyllander/rpi_plug_controller/logs"
	"github.com/mhyllander/rpi_plug_controller/server"
)

// the interval between updates if the state doesn't change
const updateInterval = time.Minute * 15

func processSensorUpdates(sensorUpdates <-chan string, done <-chan struct{}) {
	for {
		select {
		case sensorId := <-sensorUpdates:
			slog.Debug("processing state updates for sensor", "sensorId", sensorId)
			sensors, err := db.GetPlugsRelatedToSensor(sensorId)
			if err != nil {
				slog.Error("failed ot get states for plug", "plugId", sensorId, "err", err)
				break
			}
			if len(sensors) == 0 {
				slog.Info("no plugs for sensor", "sensorId", sensorId)
				break
			}
			// check each plug related to the sensor
			for _, s := range sensors {
				// update the desired plug state
				cur, err := strconv.ParseFloat(s.Value, 32)
				if err != nil {
					slog.Error("failed to convert to float", "value", s.Value, "err", err, "sensorId", s.SensorId, "plugId", s.PlugId)
					continue
				}
				min, err := strconv.ParseFloat(s.Min, 32)
				if err != nil {
					slog.Error("failed to convert to float", "min", s.Min, "err", err, "sensorId", s.SensorId, "plugId", s.PlugId)
					continue
				}
				max, err := strconv.ParseFloat(s.Max, 32)
				if err != nil {
					slog.Error("failed to convert to float", "max", s.Max, "err", err, "sensorId", s.SensorId, "plugId", s.PlugId)
					continue
				}
				newState := s.State
				switch {
				case cur > max:
					newState = "on"
				case cur < min:
					newState = "off"
				case newState == "":
					newState = "off"
				}
				if s.State != newState {
					s.State = newState
					if _, err := db.UpdateSensorPlugState(s.SensorId, s.PlugId, newState); err != nil {
						slog.Error("failed to update sensor plug state", "sensorId", s.SensorId, "plugId", s.PlugId, "err", err)
						continue
					}
				}

				// Check if any sensor wants the plug to be on
				sensorStates, err := db.GetSensorStatesForPlug(s.PlugId)
				if err != nil {
					slog.Error("failed to get desired states for plug", "plugId", s.PlugId, "err", err)
					continue
				}
				desiredState := "off"
				for _, sps := range sensorStates {
					if sps.State == "on" {
						desiredState = "on"
						break
					}
				}
				// get the plug
				plug, err := db.GetPlug(s.PlugId)
				if err != nil {
					slog.Error("failed to get plug", "plugId", s.PlugId, "err", err)
					break
				}
				// update the plug if needed
				if plug.State != desiredState || time.Since(plug.UpdatedAt) > updateInterval {
					slog.Info("notify plug", "plugId", plug.PlugId, "state", desiredState, "ipAddress", plug.IpAddress)
					resp, err := http.Get(fmt.Sprintf("http://%s/relay/0?turn=%s", plug.IpAddress, desiredState))
					var respMsg string
					if err != nil {
						slog.Warn("failed to notify plug", "plugId", plug.PlugId, "err", err)
						respMsg = err.Error()
					} else {
						slog.Debug("notify plug response", "resp", resp)
						respMsg = resp.Status
					}
					if _, err := db.UpdatePlugState(plug.PlugId, desiredState, respMsg); err != nil {
						slog.Warn("failed to update plug", "plugId", plug.PlugId, "err", err)
					}
				}
			}

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

	logs.InitLogger(*vLogLevel)

	// open and initialize database
	db.Initialize(*vDb)
	defer db.Close()

	done := make(chan struct{})
	defer close(done)

	sensorUpdates := make(chan string)
	defer close(sensorUpdates)

	go processSensorUpdates(sensorUpdates, done)

	server.StartServer(sensorUpdates, *vLogLevel)
}
