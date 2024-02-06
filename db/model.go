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
}

type Plug struct {
	gorm.Model
	PlugId    string
	IpAddress string
	State     string
}
