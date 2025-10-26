package db

import (
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var myDb *gorm.DB

func GetDBPath() string {
	db := os.Getenv("PLUGS_DB")
	if db == "" {
		db = "plugs.db"
	}
	return db
}

func Initialize(dbFile string) error {
	var err error
	myDb, err = gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// Migrate the schema
	myDb.AutoMigrate(&Sensor{}, &Plug{})

	return nil
}

func Close() {

}

// -----------------------------------------------------------------------------
// Sensor CRUD

func CreateSensorPlug(sensorId, plugId string, min, max float32) (*Sensor, error) {
	var sensor Sensor
	if result := myDb.Where(&Sensor{SensorId: sensorId, PlugId: plugId}).FirstOrInit(&sensor); result.Error != nil {
		return nil, result.Error
	}
	sensor.Min = min
	sensor.Max = max
	if result := myDb.Save(&sensor); result.Error != nil {
		return nil, result.Error
	}
	return &sensor, nil
}

// Updates the value in all sensors with sensorId.
func UpdateSensorValue(sensorId, ipAddress string, value float32) error {
	var sensors []Sensor
	if result := myDb.Where(&Sensor{SensorId: sensorId}).Find(&sensors); result.Error != nil {
		return result.Error
	}
	for _, sensor := range sensors {
		sensor.IpAddress = ipAddress
		sensor.Value = value
		if result := myDb.Save(&sensor); result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// Update the sensor state for a plug.
func UpdateSensorStateForPlug(sensorId, plugId, state string) (*Sensor, error) {
	var sensor Sensor
	if result := myDb.Where(&Sensor{SensorId: sensorId, PlugId: plugId}).FirstOrInit(&sensor); result.Error != nil {
		return nil, result.Error
	}
	sensor.State = state
	if result := myDb.Save(&sensor); result.Error != nil {
		return nil, result.Error
	}
	return &sensor, nil
}

// Get all sensors' states for a plug. Useful for updating plug state when a sensor sends an update.
func GetSensorStatesForPlug(plugId string) ([]Sensor, error) {
	var sensors []Sensor
	if result := myDb.Where(&Sensor{PlugId: plugId}).Select("State").Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	return sensors, nil
}

// Get all sensors for a sensorId. Useful for updating plug states when a sensor is updated or deleted.
func GetSensorsById(sensorId string) ([]Sensor, error) {
	var sensors []Sensor
	if result := myDb.Where(&Sensor{SensorId: sensorId}).Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	return sensors, nil
}

func GetAllSensors() ([]Sensor, error) {
	var sensors []Sensor
	if result := myDb.Order("sensor_id").Order("plug_id").Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	return sensors, nil
}

func DeleteSensor(sensorId, plugId string) error {
	if result := myDb.Where(&Sensor{SensorId: sensorId, PlugId: plugId}).Delete(&Sensor{}); result.Error != nil {
		return result.Error
	}
	return nil
}

// -----------------------------------------------------------------------------
// Plug CRUD

func CreatePlug(plugId, ipAddress string) (*Plug, error) {
	var plug Plug
	if result := myDb.Where(&Plug{PlugId: plugId}).FirstOrInit(&plug); result.Error != nil {
		return nil, result.Error
	}
	plug.IpAddress = ipAddress
	if result := myDb.Save(&plug); result.Error != nil {
		return nil, result.Error
	}
	return &plug, nil
}

func UpdatePlugState(plugId, state, response string) (*Plug, error) {
	var plug Plug
	if result := myDb.Model(&plug).Where(&Plug{PlugId: plugId}).Updates(&Plug{State: state, Response: response}); result.Error != nil {
		return nil, result.Error
	}
	return &plug, nil
}

func GetPlug(plugId string) (*Plug, error) {
	var plug Plug
	result := myDb.First(&plug, Plug{PlugId: plugId})
	if result.Error != nil {
		return nil, result.Error
	}
	return &plug, nil
}

func GetAllPlugs() ([]Plug, error) {
	var plugs []Plug
	result := myDb.Order("plug_id").Find(&plugs)
	if result.Error != nil {
		return nil, result.Error
	}
	return plugs, nil
}

func DeletePlug(plugId string) error {
	if result := myDb.Where(&Plug{PlugId: plugId}).Delete(&Plug{}); result.Error != nil {
		return result.Error
	}
	return nil
}
