package main

import (
	"log"
)

func main() {

	var config = Config{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	}

	if err := Start(config); err != nil {
		log.Println(err)
	}
}
