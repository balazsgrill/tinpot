module github.com/balazsgrill/tinpot/worker

go 1.25.5

require (
	github.com/eclipse/paho.mqtt.golang v1.5.1
	github.com/google/uuid v1.6.0
	go.nhat.io/cpy/v3 v3.12.0 // version is intentional to match python version compatibility
	go.nhat.io/python/v3 v3.12.0 // version is intentional to match python version compatibility
	github.com/balazsgrill/tinpot v0.0.0-20260112114307-6f6f6f6f6f6f
)

replace github.com/balazsgrill/tinpot => ../../tinpot

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	go.nhat.io/once v0.3.0 // indirect
	golang.org/x/net v0.44.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
)
