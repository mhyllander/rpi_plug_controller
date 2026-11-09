package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/mhyllander/rpi_plug_controller/db"
	"github.com/mhyllander/rpi_plug_controller/web"
)

const (
	listenAddress   = ":3333"
	requestTimeout  = 30 * time.Second
	shutdownTimeout = 10 * time.Second
)

// Notifier is told about changes that require plugs to be evaluated.
type Notifier interface {
	SensorUpdated(ctx context.Context, sensorId string)
	PlugChanged(ctx context.Context, plugId string)
}

type Server struct {
	store        *db.Store
	notifier     Notifier
	rootTemplate *template.Template
}

type WebSensor struct {
	SensorId  string
	IpAddress string
	PlugId    string
	State     string
	Value     string
	Min       string
	Max       string
	MinInput  string // full precision, for the threshold form
	MaxInput  string
	UpdatedAt string
}

type WebPlug struct {
	PlugId    string
	IpAddress string
	State     string
	Response  string
	UpdatedAt string
}

type RootData struct {
	PageTitle string
	Sensors   []WebSensor
	Plugs     []WebPlug
}

func NoBreak(s string) template.HTML {
	for _, ch := range []string{":", "-", ".", ","} {
		s = strings.ReplaceAll(s, ch, ch+"&NoBreak;")
	}
	return template.HTML(s)
}

func New(store *db.Store, notifier Notifier) *Server {
	webFunctions := template.FuncMap{
		"NoBreak": NoBreak,
	}
	return &Server{
		store:        store,
		notifier:     notifier,
		rootTemplate: template.Must(template.New("root.gohtml").Funcs(webFunctions).Parse(web.RootTemplate)),
	}
}

// Parses a finite floating point number.
func parseFiniteFloat32(s string) (float32, error) {
	value, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("not a finite number")
	}
	return float32(value), nil
}

func (s *Server) getRoot(w http.ResponseWriter, r *http.Request) {
	sensors, err := s.store.GetAllSensors()
	if err != nil {
		slog.Error("failed to get all sensors", "err", err)
		http.Error(w, "failed to get sensors", http.StatusInternalServerError)
		return
	}
	webSensors := make([]WebSensor, 0, len(sensors))
	for _, sensor := range sensors {
		webSensors = append(webSensors, WebSensor{
			SensorId:  sensor.SensorId,
			IpAddress: sensor.IpAddress,
			PlugId:    sensor.PlugId,
			State:     sensor.State,
			Value:     fmt.Sprintf("%0.1f", sensor.Value),
			Min:       fmt.Sprintf("%0.1f", sensor.Min),
			Max:       fmt.Sprintf("%0.1f", sensor.Max),
			MinInput:  strconv.FormatFloat(float64(sensor.Min), 'f', -1, 32),
			MaxInput:  strconv.FormatFloat(float64(sensor.Max), 'f', -1, 32),
			UpdatedAt: sensor.UpdatedAt.Format(time.DateTime),
		})
	}

	plugs, err := s.store.GetAllPlugs()
	if err != nil {
		slog.Error("failed to get all plugs", "err", err)
		http.Error(w, "failed to get plugs", http.StatusInternalServerError)
		return
	}
	webPlugs := make([]WebPlug, 0, len(plugs))
	for _, plug := range plugs {
		webPlugs = append(webPlugs, WebPlug{
			PlugId:    plug.PlugId,
			IpAddress: plug.IpAddress,
			State:     plug.State,
			Response:  plug.Response,
			UpdatedAt: plug.UpdatedAt.Format(time.DateTime),
		})
	}

	data := RootData{
		PageTitle: "Plugs Controller",
		Sensors:   webSensors,
		Plugs:     webPlugs,
	}

	// render to a buffer, so that a failure can still be reported as an error
	var page bytes.Buffer
	if err := s.rootTemplate.Execute(&page, data); err != nil {
		slog.Error("failed to render root page", "err", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page.Bytes())
}

// Validates and stores the thresholds of a sensor/plug pair, and notifies the
// controller so that the new thresholds take effect immediately.
// Returns the HTTP status code to respond with, and an error message for
// client errors.
func (s *Server) setThresholds(ctx context.Context, sensorId, plugId, minParam, maxParam string) (int, string) {
	slog.Info("update sensor plug", "sensorId", sensorId, "plugId", plugId, "min", minParam, "max", maxParam)

	minValue, err := parseFiniteFloat32(minParam)
	if err != nil {
		slog.Error("failed to convert to float", "min", minParam, "err", err)
		return http.StatusBadRequest, "min must be a number"
	}
	maxValue, err := parseFiniteFloat32(maxParam)
	if err != nil {
		slog.Error("failed to convert to float", "max", maxParam, "err", err)
		return http.StatusBadRequest, "max must be a number"
	}
	if minValue > maxValue {
		return http.StatusBadRequest, "min must not be greater than max"
	}

	if err := s.store.UpsertSensorPlug(sensorId, plugId, minValue, maxValue); err != nil {
		slog.Error("failed to update sensor plug", "sensorId", sensorId, "plugId", plugId, "err", err)
		return http.StatusInternalServerError, ""
	}

	s.notifier.SensorUpdated(ctx, sensorId)

	return http.StatusNoContent, ""
}

func (s *Server) handleUpdateSensorPlug(w http.ResponseWriter, r *http.Request) {
	status, message := s.setThresholds(
		r.Context(),
		chi.URLParam(r, "sensorId"),
		chi.URLParam(r, "plugId"),
		chi.URLParam(r, "min"),
		chi.URLParam(r, "max"),
	)
	if message != "" {
		http.Error(w, message, status)
		return
	}
	w.WriteHeader(status)
}

// Handles the threshold form on the status page.
func (s *Server) handleUpdateSensorPlugForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	status, message := s.setThresholds(
		r.Context(),
		chi.URLParam(r, "sensorId"),
		chi.URLParam(r, "plugId"),
		r.PostForm.Get("min"),
		r.PostForm.Get("max"),
	)
	if message != "" {
		http.Error(w, message, status)
		return
	}
	if status != http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleUpdateSensorValue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ipAddress := middleware.GetClientIP(ctx)
	sensorId := chi.URLParam(r, "sensorId")
	value := chi.URLParam(r, "value")
	slog.Info("update sensor value", "sensorId", sensorId, "ipAddress", ipAddress, "value", value)

	valueF, err := parseFiniteFloat32(value)
	if err != nil {
		slog.Error("failed to convert to float", "value", value, "err", err)
		http.Error(w, "value must be a number", http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateSensorValue(sensorId, ipAddress, valueF); err != nil {
		slog.Error("failed to update sensor value", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	s.notifier.SensorUpdated(ctx, sensorId)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSensor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sensorId := chi.URLParam(r, "sensorId")
	plugId := chi.URLParam(r, "plugId") // optional, "" if not provided
	slog.Info("delete sensor", "sensorId", sensorId, "plugId", plugId)

	// Collect the plugIds that are affected by deleting the sensor.
	// If no specific plugId has been provided, we will be updating all
	// related plugs, and must get them before deleting the sensor.
	plugIds := make([]string, 0, 10)
	if plugId != "" {
		plugIds = append(plugIds, plugId)
	} else {
		sensors, err := s.store.GetSensorsById(sensorId)
		if err != nil {
			slog.Error("failed to get plugs for sensor", "sensorId", sensorId, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, sensor := range sensors {
			plugIds = append(plugIds, sensor.PlugId)
		}
	}

	if err := s.store.DeleteSensor(sensorId, plugId); err != nil {
		slog.Error("failed to delete sensor", "sensorId", sensorId, "plugId", plugId, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// the affected plugs must be re-evaluated without the sensor
	for _, affectedPlugId := range plugIds {
		s.notifier.PlugChanged(ctx, affectedPlugId)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUpdatePlug(w http.ResponseWriter, r *http.Request) {
	plugId := chi.URLParam(r, "plugId")
	ipAddress := chi.URLParam(r, "ipAddress")
	slog.Info("update plug", "plugId", plugId, "ipAddress", ipAddress)

	if _, err := netip.ParseAddr(ipAddress); err != nil {
		http.Error(w, "ipAddress must be an IP address", http.StatusBadRequest)
		return
	}

	if err := s.store.UpsertPlug(plugId, ipAddress); err != nil {
		slog.Error("failed to update plug", "plugId", plugId, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeletePlug(w http.ResponseWriter, r *http.Request) {
	plugId := chi.URLParam(r, "plugId")
	slog.Info("delete plug", "plugId", plugId)

	if err := s.store.DeletePlug(plugId); err != nil {
		slog.Error("failed to delete plug", "plugId", plugId, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(httplog.RequestLogger(slog.Default(), &httplog.Options{}))
	r.Use(middleware.Recoverer)

	// Set a timeout value on the request context (ctx), that will signal
	// through ctx.Done() that the request has timed out and further
	// processing should be stopped.
	r.Use(middleware.Timeout(requestTimeout))

	// status page
	r.Get("/", s.getRoot)

	// Sensor API
	r.Route("/sensor/{sensorId}", func(r chi.Router) {
		r.Get("/plug/{plugId}/min/{min}/max/{max}", s.handleUpdateSensorPlug)
		r.Post("/plug/{plugId}/thresholds", s.handleUpdateSensorPlugForm)
		r.Get("/value/{value}", s.handleUpdateSensorValue)
		r.Delete("/{plugId}", s.handleDeleteSensor)
		r.Delete("/", s.handleDeleteSensor)
	})

	// Plug API
	r.Route("/plug/{plugId}", func(r chi.Router) {
		r.Get("/ip/{ipAddress}", s.handleUpdatePlug)
		r.Delete("/", s.handleDeletePlug)
	})

	return r
}

// ListenAndServe serves requests until ctx is cancelled, or the server fails.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              listenAddress,
		Handler:           s.routes(),
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      requestTimeout + 5*time.Second,
		IdleTimeout:       time.Minute,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	slog.Info("server closed")
	return nil
}
