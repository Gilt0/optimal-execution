package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

func main() {
	const ticker = "btcusdt"
	// GetTrades(ticker)
	GetOrderBooks(ticker)
}

const WS_ENDPOINT = "wss://stream.binance.com:9443/ws/"

type BinanceMessage struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     int      `json:"id"`
}

type TradeResponse struct {
	E  string `json:"e"`
	Ev uint64 `json:"E"`
	S  string `json:"s"`
	T  uint64 `json:"t"`
	P  string `json:"p"`
	Q  string `json:"q"`
	B  int64  `json:"b"`
	A  int64  `json:"a"`
	Tm uint64 `json:"T"`
	M  bool   `json:"m"`
	Mt bool   `json:"M"`
}

func GetTrades(ticker string) {
	headers := http.Header{}
	var binanceEndpoint = WS_ENDPOINT + ticker + "@trade"
	conn, _, err := websocket.DefaultDialer.Dial(binanceEndpoint, headers)
	if err != nil {
		log.Fatal("Error connecting to Binance WebSocket: ", err)
	}
	defer conn.Close()

	// Subscribe to <ticker>@trade
	message := BinanceMessage{
		Method: "SUBSCRIBE",
		Params: []string{ticker + "@trade"},
		ID:     1,
	}
	conn.WriteJSON(message)

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message: ", err)
			return
		}

		var response TradeResponse
		if err := json.Unmarshal(p, &response); err != nil {
			log.Println("Error unmarshalling response: ", err)
			continue
		}

		fmt.Printf("Trade: Price=%v at Time=%v\n", response.P, response.Tm)
	}
}

type OrderBookResponse struct {
	LastUpdateID int64       `json:"lastUpdateId"`
	Bids         [][2]string `json:"bids"`
	Asks         [][2]string `json:"asks"`
}

func GetOrderBooks(ticker string) {
	headers := http.Header{}
	var binanceEndpoint = WS_ENDPOINT + ticker + "@depth20"
	conn, _, err := websocket.DefaultDialer.Dial(binanceEndpoint, headers)
	if err != nil {
		log.Fatal("Error connecting to Binance WebSocket: ", err)
	}
	defer conn.Close()

	// Subscribe to <ticker>@depth20
	message := BinanceMessage{
		Method: "SUBSCRIBE",
		Params: []string{ticker + "@depth20"},
		ID:     1,
	}
	conn.WriteJSON(message)

	for {
		_, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error reading message: ", err)
			return
		}

		var response OrderBookResponse
		if err := json.Unmarshal(p, &response); err != nil {
			log.Println("Error unmarshalling response: ", err)
			continue
		}

		for _, bid := range response.Bids {
			fmt.Printf("Bid: Price=%v, Quantity=%v\n", bid[0], bid[1])
		}

		for _, ask := range response.Asks {
			fmt.Printf("Ask: Price=%v, Quantity=%v\n", ask[0], ask[1])
		}
	}
}
