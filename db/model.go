package db

import "time"

type Sensor struct {
	ID        uint `gorm:"primaryKey"`
	UpdatedAt time.Time
	SensorId  string `gorm:"uniqueIndex:idx_sensor_plug"`
	PlugId    string `gorm:"uniqueIndex:idx_sensor_plug"`
	IpAddress string
	State     string
	Value     float32
	Min       float32
	Max       float32
}

type Plug struct {
	ID        uint `gorm:"primaryKey"`
	UpdatedAt time.Time
	PlugId    string `gorm:"uniqueIndex"`
	IpAddress string
	State     string
	Response  string
}
