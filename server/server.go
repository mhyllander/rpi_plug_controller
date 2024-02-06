package server

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v2"
	"github.com/mhyllander/rpi_plug_controller/db"
	"github.com/mhyllander/rpi_plug_controller/utils"
)

var rootTemplate *template.Template

type contextKey string

const plugUpdatesKey contextKey = "plugUpdates"

type WebSensor struct {
	SensorId  string
	IpAddress string
	PlugId    string
	State     string
	UpdatedAt string
}

type WebPlug struct {
	PlugId    string
	IpAddress string
	State     string
	UpdatedAt string
}

type RootData struct {
	PageTitle string
	Sensors   []WebSensor
	Plugs     []WebPlug
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	// ctx := r.Context()

	slog.Debug("GET /")
	sensors, err := db.GetAllSensors()
	if err != nil {
		slog.Error("failed to get all sensors", "err", err)
	}
	webSensors := make([]WebSensor, 0, 10)
	for _, s := range sensors {
		webSensors = append(webSensors, WebSensor{SensorId: s.SensorId, IpAddress: s.IpAddress, PlugId: s.PlugId, State: s.State, UpdatedAt: s.UpdatedAt.Format(time.DateTime)})
	}

	plugs, err := db.GetAllPlugs()
	if err != nil {
		slog.Error("failed to get all plugs", "err", err)
	}
	webPlugs := make([]WebPlug, 0, 10)
	for _, p := range plugs {
		webPlugs = append(webPlugs, WebPlug{PlugId: p.PlugId, IpAddress: p.IpAddress, State: p.State, UpdatedAt: p.UpdatedAt.Format(time.DateTime)})
	}

	data := RootData{
		PageTitle: "Plugs Controller",
		Sensors:   webSensors,
		Plugs:     webPlugs,
	}
	rootTemplate.Execute(w, data)
}

func handleUpdateSensor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var plugUpdates chan<- string = ctx.Value(plugUpdatesKey).(chan<- string)

	ipAddress := strings.Split(r.RemoteAddr, ":")[0]
	sensorId := chi.URLParam(r, "sensorId")
	plugId := chi.URLParam(r, "plugId")
	state := chi.URLParam(r, "state")
	slog.Debug("POST /sensor", "ipAddress", ipAddress, "sensorId", sensorId, "plugId", plugId, "state", state)

	switch state {
	case "on", "off":
	default:
		slog.Error("bad state", "state", state)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if _, err := db.UpdateSensor(sensorId, plugId, state, ipAddress); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	// send the plugId for further processing
	plugUpdates <- plugId

	w.WriteHeader(http.StatusNoContent)
}

func handleDeleteSensor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var plugUpdates chan<- string = ctx.Value(plugUpdatesKey).(chan<- string)

	sensorId := chi.URLParam(r, "sensorId")
	slog.Debug("DELETE /sensor", "sensorId", sensorId)

	plugs, err := db.GetPlugsRelatedToSensor(sensorId)
	if err != nil {
		slog.Error("failed to get plugs for sensor", "sensorId", sensorId)
	}
	if err := db.DeleteSensor(sensorId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	// send the plugIds for further processing
	for _, plug := range plugs {
		plugUpdates <- plug.PlugId
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleUpdatePlug(w http.ResponseWriter, r *http.Request) {
	plugId := chi.URLParam(r, "plugId")
	ipAddress := chi.URLParam(r, "ipAddress")
	slog.Debug("POST /plug", "plugId", plugId, "ipAddress", ipAddress)
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

func StartServer(plugUpdates chan<- string, logLevel string) {
	var err error

	r := chi.NewRouter()

	// Logger
	logger := httplog.NewLogger("plugs-controller", httplog.Options{
		JSON:            true,
		LogLevel:        utils.SetLoggerOpts(logLevel).Level.Level(),
		Concise:         true,
		RequestHeaders:  false,
		SourceFieldName: "",
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
	rootTemplate = template.Must(template.ParseFiles("root.html"))

	// Sensor API
	// Custom middleware to add plugUpdatesKey
	plugUpdatesMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), plugUpdatesKey, plugUpdates)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	r.Route("/sensor/{sensorId}", func(r chi.Router) {
		r.Use(plugUpdatesMiddleware)
		r.Get("/{plugId}/{state}", handleUpdateSensor)
		r.Post("/{plugId}/{state}", handleUpdateSensor)
		r.Delete("/", handleDeleteSensor)
	})

	// Plug API
	r.Route("/plug/{plugId}", func(r chi.Router) {
		r.Get("/{ipAddress}", handleUpdatePlug)
		r.Post("/{ipAddress}", handleUpdatePlug)
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
