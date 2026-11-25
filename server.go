package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	redisKey = "canvas:512x512"
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
	return http.FileServer(http.Dir("./dist"))
}

func (s *server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Websocket upgrade error:", err)
		return
	}

	s.mutex.Lock()
	s.clients[conn] = true
	s.mutex.Unlock()

	log.Println("Connection succsesfully")

	defer func() {
		s.mutex.Lock()
		delete(s.clients, conn)
		s.mutex.Unlock()

		conn.Close()
		log.Println("Connection lost")
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
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

	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(pixel); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if pixel.Color == "" {
		http.Error(w, "Missing pixel fields", http.StatusBadRequest)
		return
	}

	if pixel.X < 0 || pixel.X >= 512 ||
		pixel.Y < 0 || pixel.Y >= 512 {
		http.Error(w, "Invalid coordinates", http.StatusBadRequest)
		return
	}

	if !isValidColor(pixel.Color) {
		http.Error(w, "Invalid color", http.StatusBadRequest)
		return
	}

	field := fmt.Sprintf("%d:%d", pixel.X, pixel.Y)

	if err := s.restore.HSet(r.Context(), redisKey, field, pixel.Color).Err(); err != nil {
		log.Println("Redis HSet error:", err)
		http.Error(w, "Failed to save pixel", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	s.broadcastPixel(pixel)
}

func (s *server) broadcastPixel(pixel *PixelRequest) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for conn := range s.clients {
		if err := conn.WriteJSON(pixel); err != nil {
			log.Println(err)
			delete(s.clients, conn)
			conn.Close()
		}
	}
}

func isValidColor(color string) bool {
	matched, _ := regexp.MatchString(
		`^#[0-9a-fA-F]{6}$`,
		color,
	)

	return matched
}
