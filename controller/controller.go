package controller

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/mhyllander/rpi_plug_controller/db"
)

const (
	stateOn  = "on"
	stateOff = "off"

	// the interval between updates if the state doesn't change
	updateInterval = time.Minute * 60

	plugRequestTimeout = time.Second * 5

	// allows the HTTP handlers to queue updates while a plug is being switched
	updateQueueSize = 100
)

// Controller turns plugs on and off based on sensor readings.
type Controller struct {
	store         *db.Store
	httpClient    *http.Client
	plugPort      string
	sensorUpdates chan string
	plugUpdates   chan string
}

func New(store *db.Store, plugPort string) *Controller {
	return &Controller{
		store:         store,
		httpClient:    &http.Client{Timeout: plugRequestTimeout},
		plugPort:      plugPort,
		sensorUpdates: make(chan string, updateQueueSize),
		plugUpdates:   make(chan string, updateQueueSize),
	}
}

// SensorUpdated queues the plugs of a sensor with a new value for evaluation.
func (c *Controller) SensorUpdated(ctx context.Context, sensorId string) {
	select {
	case c.sensorUpdates <- sensorId:
	case <-ctx.Done():
	}
}

// PlugChanged queues a plug for evaluation, e.g. after one of its sensors was deleted.
func (c *Controller) PlugChanged(ctx context.Context, plugId string) {
	select {
	case c.plugUpdates <- plugId:
	case <-ctx.Done():
	}
}

// Run processes updates until ctx is cancelled.
func (c *Controller) Run(ctx context.Context) {
	for {
		select {
		case sensorId := <-c.sensorUpdates:
			c.processSensorUpdate(sensorId)
		case plugId := <-c.plugUpdates:
			c.evaluatePlug(plugId)
		case <-ctx.Done():
			return
		}
	}
}

func (c *Controller) processSensorUpdate(sensorId string) {
	slog.Debug("processing state updates for sensor", "sensorId", sensorId)
	sensors, err := c.store.GetSensorsById(sensorId)
	if err != nil {
		slog.Error("failed to get plugs for sensor", "sensorId", sensorId, "err", err)
		return
	}
	if len(sensors) == 0 {
		slog.Info("no plugs for sensor", "sensorId", sensorId)
		return
	}

	for _, sensor := range sensors {
		newState := nextSensorState(sensor.Value, sensor.Min, sensor.Max, sensor.State)
		if sensor.State != newState {
			if err := c.store.UpdateSensorState(sensor.SensorId, sensor.PlugId, newState); err != nil {
				slog.Error("failed to update desired plug state", "sensorId", sensor.SensorId, "plugId", sensor.PlugId, "err", err)
				continue
			}
		}
		c.evaluatePlug(sensor.PlugId)
	}
}

// Switches the plug if its desired state differs from its current state, or if it hasn't been updated for a while.
func (c *Controller) evaluatePlug(plugId string) {
	plug, err := c.store.GetPlug(plugId)
	if err != nil {
		slog.Error("failed to get plug", "plugId", plugId, "err", err)
		return
	}

	sensors, err := c.store.GetSensorStatesForPlug(plugId)
	if err != nil {
		slog.Error("failed to get desired states for plug", "plugId", plugId, "err", err)
		return
	}
	desiredState := desiredPlugState(sensors)

	if plug.State == desiredState && time.Since(plug.UpdatedAt) <= updateInterval {
		return
	}

	slog.Info("notify plug", "plugId", plug.PlugId, "state", desiredState, "ipAddress", plug.IpAddress)
	status, err := c.switchPlug(plug.IpAddress, desiredState)
	if err != nil {
		slog.Warn("failed to notify plug", "plugId", plug.PlugId, "err", err)
		if err := c.store.UpdatePlugResponse(plug.PlugId, err.Error()); err != nil {
			slog.Warn("failed to update plug", "plugId", plug.PlugId, "err", err)
		}
		return
	}
	if err := c.store.UpdatePlugState(plug.PlugId, desiredState, status); err != nil {
		slog.Warn("failed to update plug", "plugId", plug.PlugId, "err", err)
	}
}

// Returns the HTTP status of the plug's response.
func (c *Controller) switchPlug(ipAddress, state string) (string, error) {
	plugUrl := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(ipAddress, c.plugPort),
		Path:     "/relay/0",
		RawQuery: url.Values{"turn": {state}}.Encode(),
	}
	resp, err := c.httpClient.Get(plugUrl.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	slog.Debug("notify plug response", "status", resp.Status)
	if resp.StatusCode != http.StatusOK {
		return resp.Status, fmt.Errorf("unexpected response: %s", resp.Status)
	}
	return resp.Status, nil
}

// Returns the state a sensor wants its plug to be in. Between min and max the
// state is unchanged, and initially off.
func nextSensorState(value, minValue, maxValue float32, currentState string) string {
	switch {
	case value > maxValue:
		return stateOn
	case value <= minValue:
		return stateOff
	case currentState == "":
		return stateOff
	default:
		return currentState
	}
}

// A plug is on if any of its sensors wants it on.
func desiredPlugState(sensors []db.Sensor) string {
	for _, sensor := range sensors {
		if sensor.State == stateOn {
			return stateOn
		}
	}
	return stateOff
}
