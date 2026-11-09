package db

import (
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type Store struct {
	gormDB *gorm.DB
}

func GetDBPath() string {
	db := os.Getenv("PLUGS_DB")
	if db == "" {
		db = "plugs.db"
	}
	return db
}

func Open(dbFile string) (*Store, error) {
	gormDB, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbFile, err)
	}

	store := &Store{gormDB: gormDB}
	if err := store.migrate(); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	sqlDB, err := s.gormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) migrate() error {
	// Earlier versions embedded gorm.Model, which soft deletes rows. Purge
	// those rows so that the unique indexes can be created.
	migrator := s.gormDB.Migrator()
	for _, table := range []string{"sensors", "plugs"} {
		if migrator.HasTable(table) && migrator.HasColumn(table, "deleted_at") {
			if err := s.gormDB.Exec("DELETE FROM " + table + " WHERE deleted_at IS NOT NULL").Error; err != nil {
				return fmt.Errorf("purge deleted rows from %s: %w", table, err)
			}
		}
	}

	if err := s.gormDB.AutoMigrate(&Sensor{}, &Plug{}); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	return nil
}

// -----------------------------------------------------------------------------
// Sensor CRUD

// Links a sensor to a plug, or updates the thresholds of an existing link.
func (s *Store) UpsertSensorPlug(sensorId, plugId string, minValue, maxValue float32) error {
	var sensor Sensor
	err := s.gormDB.
		Where("sensor_id = ? AND plug_id = ?", sensorId, plugId).
		Attrs(Sensor{SensorId: sensorId, PlugId: plugId}).
		FirstOrInit(&sensor).Error
	if err != nil {
		return err
	}

	sensor.Min = minValue
	sensor.Max = maxValue
	return s.gormDB.Save(&sensor).Error
}

// Updates the value in all sensors with sensorId.
func (s *Store) UpdateSensorValue(sensorId, ipAddress string, value float32) error {
	return s.gormDB.Model(&Sensor{}).
		Where("sensor_id = ?", sensorId).
		Updates(map[string]any{"ip_address": ipAddress, "value": value}).Error
}

// Update the sensor state for a plug.
func (s *Store) UpdateSensorState(sensorId, plugId, state string) error {
	return s.gormDB.Model(&Sensor{}).
		Where("sensor_id = ? AND plug_id = ?", sensorId, plugId).
		Update("state", state).Error
}

// Get all sensors' states for a plug. Useful for updating plug state when a sensor sends an update.
func (s *Store) GetSensorStatesForPlug(plugId string) ([]Sensor, error) {
	var sensors []Sensor
	if result := s.gormDB.Where("plug_id = ?", plugId).Select("state").Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	return sensors, nil
}

// Get all sensors for a sensorId. Useful for updating plug states when a sensor is updated or deleted.
func (s *Store) GetSensorsById(sensorId string) ([]Sensor, error) {
	var sensors []Sensor
	if result := s.gormDB.Where("sensor_id = ?", sensorId).Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	return sensors, nil
}

func (s *Store) GetAllSensors() ([]Sensor, error) {
	var sensors []Sensor
	if result := s.gormDB.Order("sensor_id").Order("plug_id").Find(&sensors); result.Error != nil {
		return nil, result.Error
	}
	return sensors, nil
}

// Deletes the link between a sensor and a plug, or all links of the sensor if plugId is empty.
func (s *Store) DeleteSensor(sensorId, plugId string) error {
	query := s.gormDB.Where("sensor_id = ?", sensorId)
	if plugId != "" {
		query = query.Where("plug_id = ?", plugId)
	}
	return query.Delete(&Sensor{}).Error
}

// -----------------------------------------------------------------------------
// Plug CRUD

func (s *Store) UpsertPlug(plugId, ipAddress string) error {
	var plug Plug
	err := s.gormDB.
		Where("plug_id = ?", plugId).
		Attrs(Plug{PlugId: plugId}).
		FirstOrInit(&plug).Error
	if err != nil {
		return err
	}

	plug.IpAddress = ipAddress
	return s.gormDB.Save(&plug).Error
}

// Records a successful plug update. This also resets the plug's update time.
func (s *Store) UpdatePlugState(plugId, state, response string) error {
	return s.gormDB.Model(&Plug{}).
		Where("plug_id = ?", plugId).
		Updates(map[string]any{"state": state, "response": response}).Error
}

// Records a failed plug update. The state and update time are left unchanged,
// so that the update is retried.
func (s *Store) UpdatePlugResponse(plugId, response string) error {
	return s.gormDB.Model(&Plug{}).
		Where("plug_id = ?", plugId).
		UpdateColumn("response", response).Error
}

func (s *Store) GetPlug(plugId string) (*Plug, error) {
	var plug Plug
	if result := s.gormDB.Where("plug_id = ?", plugId).First(&plug); result.Error != nil {
		return nil, result.Error
	}
	return &plug, nil
}

func (s *Store) GetAllPlugs() ([]Plug, error) {
	var plugs []Plug
	if result := s.gormDB.Order("plug_id").Find(&plugs); result.Error != nil {
		return nil, result.Error
	}
	return plugs, nil
}

// Deletes a plug and the sensor links that refer to it.
func (s *Store) DeletePlug(plugId string) error {
	return s.gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("plug_id = ?", plugId).Delete(&Sensor{}).Error; err != nil {
			return err
		}
		return tx.Where("plug_id = ?", plugId).Delete(&Plug{}).Error
	})
}
