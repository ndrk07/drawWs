package main

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn     *websocket.Conn
	clientMu sync.Mutex
}
