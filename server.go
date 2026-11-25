package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	redisKey = "canvas:64x64"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type server struct {
	router  *http.ServeMux
	restore *redis.Client
	mutex   sync.Mutex
	clients map[*websocket.Conn]bool
}

type PixelRequest struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

func newServer(restore *redis.Client) *server {
	s := &server{
		router:  http.NewServeMux(),
		restore: restore,
		clients: make(map[*websocket.Conn]bool),
	}

	s.configurate()
	return s
}

func Start(cfg Config) error {
	redisStore, err := NewClient(context.Background(), cfg)
	if err != nil {
		return err
	}
	srv := newServer(redisStore)

	return http.ListenAndServe(":8080", srv.router)
}

func (s *server) configurate() {
	s.router.Handle("/", handleStart())
	s.router.HandleFunc("POST /api/canvas/pixels", s.handleSavePixel)
	s.router.HandleFunc("GET /api/canvas/pixels", s.handleLoadPixels)
	s.router.HandleFunc("/ws", s.handleWebSocket)
}

func handleStart() http.Handler {
	return http.FileServer(http.Dir("../../projectGo/pixel-editor/dist"))
}

func (s *server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
		return
	}

	s.mutex.Lock()
	s.clients[conn] = true
	s.mutex.Unlock()

	log.Println("Connection succsesfully")

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	s.mutex.Lock()
	s.clients[conn] = false
	s.mutex.Unlock()

	log.Println("Connection lost")
}

func (s *server) handleLoadPixels(w http.ResponseWriter, r *http.Request) {
	pixels, err := s.restore.HGetAll(r.Context(), redisKey).Result()
	if err != nil {
		http.Error(w, "Error to load pixels", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pixels)
}

func (s *server) handleSavePixel(w http.ResponseWriter, r *http.Request) {
	pixel := &PixelRequest{}

	if err := json.NewDecoder(r.Body).Decode(pixel); err != nil {
		http.Error(w, "Invalid Request", http.StatusBadRequest)
		return
	}

	field := fmt.Sprintf("%d:%d", pixel.X, pixel.Y)

	if err := s.restore.HSet(r.Context(), redisKey, field, pixel.Color).Err(); err != nil {
		fmt.Println(err)
		return
	}

	s.broadcastPixel(pixel)
}

func (s *server) broadcastPixel(pixel *PixelRequest) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for conn := range s.clients {
		if err := conn.WriteJSON(pixel); err != nil {
			log.Println(err)
			conn.Close()
			delete(s.clients, conn)
		}
	}
}
