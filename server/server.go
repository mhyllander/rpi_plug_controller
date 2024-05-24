package server

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httplog "github.com/mhyllander/go-chi-httplog/v2"
	"github.com/mhyllander/rpi_plug_controller/db"
	"github.com/mhyllander/rpi_plug_controller/logs"
)

var rootTemplate *template.Template

type contextKey string

const sensorUpdatesKey contextKey = "sensorUpdates"

type WebSensor struct {
	SensorId  string
	IpAddress string
	PlugId    string
	State     string
	Value     string
	Min       string
	Max       string
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
	for _, ch := range []string{":", "-", "."} {
		s = strings.ReplaceAll(s, ch, ch+"&NoBreak;")
	}
	return template.HTML(s)
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	slog.Debug("GET /")
	sensors, err := db.GetAllSensors()
	if err != nil {
		slog.Error("failed to get all sensors", "err", err)
	}
	webSensors := make([]WebSensor, 0, 10)
	for _, s := range sensors {
		webSensors = append(webSensors, WebSensor{SensorId: s.SensorId, IpAddress: s.IpAddress, PlugId: s.PlugId, State: s.State, Value: s.Value, Min: s.Min, Max: s.Max, UpdatedAt: s.UpdatedAt.Format(time.DateTime)})
	}

	plugs, err := db.GetAllPlugs()
	if err != nil {
		slog.Error("failed to get all plugs", "err", err)
	}
	webPlugs := make([]WebPlug, 0, 10)
	for _, p := range plugs {
		webPlugs = append(webPlugs, WebPlug{PlugId: p.PlugId, IpAddress: p.IpAddress, State: p.State, Response: p.Response, UpdatedAt: p.UpdatedAt.Format(time.DateTime)})
	}

	data := RootData{
		PageTitle: "Plugs Controller",
		Sensors:   webSensors,
		Plugs:     webPlugs,
	}
	err = rootTemplate.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleUpdateSensorPlug(w http.ResponseWriter, r *http.Request) {
	sensorId := chi.URLParam(r, "sensorId")
	plugId := chi.URLParam(r, "plugId")
	min := chi.URLParam(r, "min")
	max := chi.URLParam(r, "max")
	slog.Debug("GET /v2/sensor/*/plug/*/min/*/max", "sensorId", sensorId, "plugId", plugId, "min", min, "max", max)

	slog.Info("update sensor plug", "sensorId", sensorId, "plugId", plugId, "min", min, "max", max)

	if _, err := strconv.ParseFloat(min, 32); err != nil {
		slog.Error("failed to convert to float", "min", min, "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if _, err := strconv.ParseFloat(max, 32); err != nil {
		slog.Error("failed to convert to float", "max", max, "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if _, err := db.CreateSensorPlug(sensorId, plugId, min, max); err != nil {

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleUpdateSensorValue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var sensorUpdates chan<- string = ctx.Value(sensorUpdatesKey).(chan<- string)

	ipAddress := strings.Split(r.RemoteAddr, ":")[0]
	sensorId := chi.URLParam(r, "sensorId")
	value := chi.URLParam(r, "value")
	slog.Debug("GET /sensor/*/value", "sensorId", sensorId, "ipAddress", ipAddress, "value", value)

	slog.Info("update sensor value", "sensorId", sensorId, "ipAddress", ipAddress, "value", value)

	if err := db.UpdateSensorValue(sensorId, ipAddress, value); err != nil {
		slog.Error("failed to update sensor value", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// send the sensorId for further processing
	sensorUpdates <- sensorId

	w.WriteHeader(http.StatusNoContent)
}

func handleDeleteSensor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var sensorUpdates chan<- string = ctx.Value(sensorUpdatesKey).(chan<- string)

	sensorId := chi.URLParam(r, "sensorId")
	plugId := chi.URLParam(r, "plugId") // optional, "" if not provided
	slog.Debug("DELETE /sensor", "sensorId", sensorId, "plugId", plugId)

	// Collect the plugIds that are affected by deleting the sensor.
	// If no specifc plugId has been provided, we will be updating all
	// related plugs, and must get them before deleting the sensor.
	plugIds := make([]string, 0, 10)
	if plugId != "" {
		plugIds = append(plugIds, plugId)
	} else {
		if plugs, err := db.GetPlugsRelatedToSensor(sensorId); err == nil {
			for _, plug := range plugs {
				plugIds = append(plugIds, plug.PlugId)
			}
		} else {
			slog.Error("failed to get plugs for sensor", "sensorId", sensorId)
		}
	}

	if err := db.DeleteSensor(sensorId, plugId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	// send the plugIds for further processing
	for _, pid := range plugIds {
		sensorUpdates <- pid
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleUpdatePlug(w http.ResponseWriter, r *http.Request) {
	plugId := chi.URLParam(r, "plugId")
	ipAddress := chi.URLParam(r, "ipAddress")
	slog.Debug("GET /plug", "plugId", plugId, "ipAddress", ipAddress)
	if _, err := db.CreatePlug(plugId, ipAddress); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleDeletePlug(w http.ResponseWriter, r *http.Request) {
	plugId := chi.URLParam(r, "plugId")
	slog.Debug("DELETE /plug", "plugId", plugId)
	if err := db.DeletePlug(plugId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusNoContent)
}

func StartServer(sensorUpdates chan<- string, logLevel string) {
	var err error

	r := chi.NewRouter()

	// Logger
	logger := httplog.NewLogger("plugs-controller", httplog.Options{
		JSON:             true,
		LogLevel:         logs.SetLoggerOpts(logLevel).Level.Level(),
		Concise:          true,
		RequestHeaders:   false,
		SourceFieldName:  "",
		TimeFieldName:    "time",
		MessageFieldName: "msg",
	})

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(httplog.RequestLogger(logger))
	r.Use(middleware.Recoverer)

	// Set a timeout value on the request context (ctx), that will signal
	// through ctx.Done() that the request has timed out and further
	// processing should be stopped.
	r.Use(middleware.Timeout(60 * time.Second))

	// status page
	r.Get("/", getRoot)
	webFunctions := template.FuncMap{
		"NoBreak": NoBreak,
	}
	rootTemplate = template.Must(template.New("root.gohtml").Funcs(webFunctions).ParseFiles("web/root.gohtml"))

	// Sensor API
	// Custom middleware to add sensorUpdatesKey
	sensorUpdatesMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), sensorUpdatesKey, sensorUpdates)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	r.Route("/sensor/{sensorId}", func(r chi.Router) {
		r.Use(sensorUpdatesMiddleware)
		r.Get("/plug/{plugId}/min/{min}/max/{max}", handleUpdateSensorPlug)
		r.Get("/value/{value}", handleUpdateSensorValue)
		r.Delete("/{plugId}", handleDeleteSensor)
		r.Delete("/", handleDeleteSensor)
	})

	// Plug API
	r.Route("/plug/{plugId}", func(r chi.Router) {
		r.Get("/ip/{ipAddress}", handleUpdatePlug)
		r.Delete("/", handleDeletePlug)
	})

	err = http.ListenAndServe(":3333", r)
	if errors.Is(err, http.ErrServerClosed) {
		slog.Info("server closed")
	} else if err != nil {
		slog.Error("error starting server", "err", err)
		os.Exit(1)
	}
}
