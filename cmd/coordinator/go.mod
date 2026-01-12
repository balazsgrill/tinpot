module github.com/balazsgrill/tinpot/coordinator

go 1.25.5

require (
	github.com/balazsgrill/tinpot v0.0.0-00010101000000-000000000000
	github.com/eclipse/paho.mqtt.golang v1.5.1
	github.com/google/uuid v1.6.0
)

replace github.com/balazsgrill/tinpot => ../../tinpot

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/net v0.44.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
)
