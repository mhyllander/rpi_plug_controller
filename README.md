# RPi Plug Controller

A small service for a Raspberry Pi that switches smart plugs based on humidity
sensors. Sensors report humidity readings over HTTP. Each sensor is linked to
one or more plugs with a `min` and `max` threshold:

* value > `max`: the sensor wants the plug **on**
* value <= `min`: the sensor wants the plug **off**
* otherwise: the sensor keeps its previous state (initially off)

A plug is turned on if any linked sensor wants it on. The plug is notified
(`GET http://<plug-ip>/relay/0?turn=on|off`) when its state changes, and
refreshed every 60 minutes otherwise. A failed notification is retried with
the next sensor reading.

A status page showing all sensors and plugs is served at `/`. The `min` and
`max` thresholds of a sensor/plug pair can be edited on the page.

## Running

    make build        # bin/plugs_controller
    make build-rpi    # arm64/plugs_controller
    bin/plugs_controller [-db plugs.db] [-log-level info] [-plug-port 80]

The SQLite database defaults to `$PLUGS_DB`, or `plugs.db`. The server listens
on port 3333.

## API

All endpoints are on port 3333. Parameters are part of the path. Successful
calls return `204 No Content`. Invalid input returns `400`: a `min`, `max` or
`value` that is not a finite number, `min` greater than `max`, or an
`ipAddress` that is not an IP address. A database failure returns `500`.

### Plugs

| Method   | Path                          | Description                           |
|----------|-------------------------------|---------------------------------------|
| `GET`    | `/plug/{plugId}/ip/{ipAddress}` | Register a plug, or update its IP address |
| `DELETE` | `/plug/{plugId}`              | Delete a plug and its sensor links    |

### Sensors

| Method   | Path                                                  | Description |
|----------|-------------------------------------------------------|-------------|
| `GET`    | `/sensor/{sensorId}/plug/{plugId}/min/{min}/max/{max}` | Link a sensor to a plug with thresholds, or update them |
| `GET`    | `/sensor/{sensorId}/value/{value}`                    | Submit a humidity reading. The sensor's IP address is taken from the request. |
| `DELETE` | `/sensor/{sensorId}/{plugId}`                         | Remove the link between a sensor and a plug |
| `DELETE` | `/sensor/{sensorId}`                                  | Remove the sensor from all plugs |

Plugs affected by a deleted sensor are re-evaluated. A plug must be registered
before a sensor reading can switch it.

### Example

    curl localhost:3333/plug/fan/ip/192.168.1.50
    curl localhost:3333/sensor/bathroom/plug/fan/min/55/max/70
    curl localhost:3333/sensor/bathroom/value/72.5
    curl -X DELETE localhost:3333/sensor/bathroom/fan
