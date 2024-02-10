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
func UpdateSensor(sensorId, ipAddress, plugId, state, value string) (*Sensor, error) {
	var sensor Sensor
	if result := myDb.Where(&Sensor{SensorId: sensorId, PlugId: plugId}).Assign(&Sensor{IpAddress: ipAddress, State: state, Value: value}).FirstOrCreate(&sensor); result.Error != nil {
		return nil, result.Error
	}
	return &sensor, nil
}

func GetSensor(sensorId, plugId string) (*Sensor, error) {
	var sensor Sensor
	if result := myDb.First(&sensor, Sensor{SensorId: sensorId, PlugId: plugId}); result.Error != nil {
		return nil, result.Error
	}
	return &sensor, nil
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
	if result := myDb.Where(&Plug{PlugId: plugId}).Assign(&Plug{IpAddress: ipAddress}).FirstOrCreate(&plug); result.Error != nil {
		return nil, result.Error
	}
	return &plug, nil
}

func UpdatePlug(plugId, state, response string) (*Plug, error) {
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

//-----------------------------------------------------------------------------
// Specific queries

// Get requested states for a plug. Useful for updating plug state when a
// sensor sends an update.
func GetStatesForPlug(plugId string) ([]Sensor, error) {
	var sensors []Sensor
	if result := myDb.Where(&Sensor{PlugId: plugId}).Select("State").Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	return sensors, nil
}

// Get all plugIds related to a specific sensor. Useful for updating plug state
// when a sensor is deleted.
func GetPlugsRelatedToSensor(sensorId string) ([]Sensor, error) {
	var sensors []Sensor
	if result := myDb.Where(&Sensor{SensorId: sensorId}).Select("PlugId").Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	// TODO: convert results to an array of plug ids
	return sensors, nil
}
