package db

import (
	"gorm.io/gorm"
)

type Sensor struct {
	gorm.Model
	SensorId  string
	IpAddress string
	PlugId    string
	State     string
	Value     float32
	Min       float32
	Max       float32
}

type Plug struct {
	gorm.Model
	PlugId    string
	IpAddress string
	State     string
	Response  string
}
