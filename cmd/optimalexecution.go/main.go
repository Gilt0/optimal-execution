package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var binanceEndpoint = "wss://stream.binance.com:9443/ws/btcusdt@trade"

type BinanceMessage struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     int      `json:"id"`
}

func main() {
	headers := http.Header{}
	conn, _, err := websocket.DefaultDialer.Dial(binanceEndpoint, headers)
	if err != nil {
		log.Fatal("Error connecting to Binance WebSocket: ", err)
	}
	defer conn.Close()

	// Subscribe to btcusdt@trade
	message := BinanceMessage{
		Method: "SUBSCRIBE",
		Params: []string{"btcusdt@trade"},
		ID:     1,
	}
	conn.WriteJSON(message)

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message: ", err)
			return
		}

		var response map[string]interface{}
		if err := json.Unmarshal(p, &response); err != nil {
			log.Println("Error unmarshalling response: ", err)
			continue
		}

		fmt.Printf("Trade: Price=%v at Time=%v\n", response["p"], response["T"])
	}
}
