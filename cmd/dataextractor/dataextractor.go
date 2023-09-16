package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	WS_ENDPOINT             = "wss://stream.binance.com:9443/ws/"
	REST_ENDPOINT           = "https://api.binance.com/api/v3/depth"
	DEFAULT_JSON_PATH       = "./"
	EMPTY                   = ""
	LIMIT                   = 5000
	DEFAULT_THROTTLE        = 5
	INTERPOLATION_NB        = 100
	ORDERBOOK_PRINT_LINE_NB = 5
	BIPS                    = 10000
	QUANTILE                = 0.95
)

var (
	symbol    string
	json_path string
	throttle  int
)

func printCommandWithDefaults() {
	var executable = os.Args[0]
	var elements = strings.Split(executable, "/")
	cmd := []string{elements[len(elements)-1]}
	flag.VisitAll(func(f *flag.Flag) {
		cmd = append(cmd, fmt.Sprintf("-%s=<%s>", f.Name, f.Name))
	})
	fmt.Println("Command with placeholders:\n", strings.Join(cmd, " "))
	flag.PrintDefaults()
}

func init() {
	flag.StringVar(&symbol, "symbol", EMPTY, "Ticker symbol to fetch data for in uppercase")
	flag.StringVar(&json_path, "json_path", DEFAULT_JSON_PATH, "Path to save the data files")
	flag.IntVar(&throttle, "throttle", DEFAULT_THROTTLE, "Time interval in seconds to fetch and save data")
	flag.Parse()
	if symbol == EMPTY {
		fmt.Println("Error: The 'symbol' argument is required.")
		printCommandWithDefaults()
		os.Exit(1)
	}
	if symbol != strings.ToUpper(symbol) {
		fmt.Println("Error: The 'symbol' argument should be in uppercase.")
		printCommandWithDefaults()
		os.Exit(1)
	}
}

type TradeResponse struct {
	E  string  `json:"e"`
	Ev uint64  `json:"E"`
	S  string  `json:"s"`
	T  uint64  `json:"t"`
	P  float64 `json:"p,string"`
	Q  float64 `json:"q,string"`
	B  int64   `json:"b"`
	A  int64   `json:"a"`
	Tm uint64  `json:"T"`
	M  bool    `json:"m"`
	Mt bool    `json:"M"`
}

type AggregatedData struct {
	LastTimestamp  uint64  `json:"last_timestamp"`
	TotalAmount    float64 `json:"total_amount"`
	TotalNumber    uint64  `json:"total_number"`
	TotalVolume    float64 `json:"total_volume"`
	OrderSize      float64 `json:"order_size"`
	OrderAmount    float64 `json:"order_amount"`
	BidTotalAmount float64 `json:"bid_total_amount"`
	BidTotalNumber uint64  `json:"bid_total_number"`
	BidTotalVolume float64 `json:"bid_total_volume"`
	BidOrderSize   float64 `json:"bid_order_size"`
	BidOrderAmount float64 `json:"bid_order_amount"`
	AskTotalAmount float64 `json:"ask_total_amount"`
	AskTotalNumber uint64  `json:"ask_total_number"`
	AskTotalVolume float64 `json:"ask_total_volume"`
	AskOrderSize   float64 `json:"ask_order_size"`
	AskOrderAmount float64 `json:"ask_order_amount"`
	BidTotalQty    float64 `json:"bid_total_qty"`
	AskTotalQty    float64 `json:"ask_total_qty"`
	BidMaxDelta    float64 `json:"bid_max_delta"`
	AskMaxDelta    float64 `json:"ask_max_delta"`
	MidPrice       float64 `json:"mid_price"`
	BuyPrice       float64 `json:"buy_price"`
	SellPrice      float64 `json:"sell_price"`
	Spread         float64 `json:"spread"`
}

type OrderBookResponse struct {
	LastUpdateID int64       `json:"lastUpdateId"`
	Bids         [][2]string `json:"bids"`
	Asks         [][2]string `json:"asks"`
}

func fetchOrderBook() (*OrderBookResponse, error) {
	url := fmt.Sprintf("%s?symbol=%s&limit=%d", REST_ENDPOINT, symbol, LIMIT)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var orderBook OrderBookResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderBook); err != nil {
		return nil, err
	}
	return &orderBook, nil
}

func handleTradeStream(aggregated chan<- *TradeResponse) {
	headers := http.Header{}
	conn, _, err := websocket.DefaultDialer.Dial(WS_ENDPOINT+strings.ToLower(symbol)+"@trade", headers)
	if err != nil {
		log.Fatal("Error connecting to Binance WebSocket: ", err)
	}
	defer conn.Close()
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
		aggregated <- &response
	}
}

type OrderLevel struct {
	Price    float64
	Quantity float64
}

// Helper function to convert the OrderBookResponse levels to float64
func convertLevels(levels [][2]string) ([]OrderLevel, error) {
	var result []OrderLevel
	for _, level := range levels {
		price, err := strconv.ParseFloat(level[0], 64)
		if err != nil {
			return nil, err
		}
		quantity, err := strconv.ParseFloat(level[1], 64)
		if err != nil {
			return nil, err
		}
		result = append(result, OrderLevel{Price: price, Quantity: quantity})
	}
	return result, nil
}

type CumulatedProfile [2]float64

type ExtendedOrderBookResponse struct {
	LastUpdateID int64              `json:"lastUpdateId"`
	CumBids      []CumulatedProfile `json:"cumBids"`
	CumAsks      []CumulatedProfile `json:"cumAsks"`
}

func interpolateCumulatedProfile(profile []CumulatedProfile) []CumulatedProfile {
	// 99 points for the range 0.001 to 0.099
	fineStep := 0.01 / float64(INTERPOLATION_NB)
	remainingStep := 1.0 / float64(INTERPOLATION_NB)
	interpolated := make([]CumulatedProfile, 2*INTERPOLATION_NB+1)
	// Ensure at delta = 0, liquidity = 0
	interpolated[0] = CumulatedProfile{0, 0}
	i := 1
	// Handle the fine-grained points
	for delta_i := 0.0001; delta_i < 0.01; delta_i += fineStep {
		interpolated[i] = linearInterpolation(profile, delta_i)
		i++
		fmt.Println("i:", i)
	}
	// Handle the remaining points
	for delta_i := 0.01; delta_i < 1.0; delta_i += remainingStep {
		interpolated[i] = linearInterpolation(profile, delta_i)
		i++
		fmt.Println("i:", i)
	}
	// Ensure at delta = 1, liquidity = 1
	interpolated[2*INTERPOLATION_NB] = CumulatedProfile{1, 1}
	fmt.Println("out")
	return interpolated
}

func linearInterpolation(profile []CumulatedProfile, delta_i float64) CumulatedProfile {
	for j := 0; j < len(profile)-1; j++ {
		if profile[j][0] <= delta_i && profile[j+1][0] >= delta_i {
			// Linear interpolation formula: y = y1 + (y2-y1) * (x-x1) / (x2-x1)
			liquidity := profile[j][1] + (profile[j+1][1]-profile[j][1])*(delta_i-profile[j][0])/(profile[j+1][0]-profile[j][0])
			return CumulatedProfile{delta_i, liquidity}
		}
	}
	return CumulatedProfile{delta_i, 1.0} // Default if not found in the given profile
}

type AccumulatedData struct {
	Spread         float64
	OrderSize      float64
	OrderAmount    float64
	BidOrderSize   float64
	BidOrderAmount float64
	BidTotalQty    float64
	BidMaxDelta    float64
	AskOrderSize   float64
	AskOrderAmount float64
	AskTotalQty    float64
	AskMaxDelta    float64
	CumBids        [2*INTERPOLATION_NB + 1]CumulatedProfile
	CumAsks        [2*INTERPOLATION_NB + 1]CumulatedProfile
}

func processTrade(trade *TradeResponse, aggData *AggregatedData) {
	aggData.LastTimestamp = trade.Tm
	aggData.TotalAmount += trade.P * trade.Q
	aggData.TotalVolume += trade.Q
	aggData.TotalNumber++

	if trade.M {
		aggData.BidTotalAmount += trade.P * trade.Q
		aggData.BidTotalVolume += trade.Q
		aggData.BidTotalNumber++
	} else {
		aggData.AskTotalAmount += trade.P * trade.Q
		aggData.AskTotalVolume += trade.Q
		aggData.AskTotalNumber++
	}
}

// Computes the average execution price on the ask for an order of size orderSize
func computeBuyPrice(asks []OrderLevel, orderSize float64) float64 {
	totalAmount := 0.0
	totalQty := 0.0
	for _, ask := range asks {
		totalQty += ask.Quantity
		totalAmount += ask.Quantity * ask.Price
		if totalQty >= orderSize {
			remainingQty := totalQty - orderSize
			totalAmount -= remainingQty * ask.Price
			return totalAmount / (totalQty - remainingQty)
		}
	}
	return 0 // This indicates that there's not enough liquidity to fill the order
}

// Computes the average execution price on the bid for an order of size orderSize
func computeSellPrice(bids []OrderLevel, orderSize float64) float64 {
	totalAmount := 0.0
	totalQty := 0.0
	for _, bid := range bids {
		totalQty += bid.Quantity
		totalAmount += bid.Quantity * bid.Price
		if totalQty >= orderSize {
			remainingQty := totalQty - orderSize
			totalAmount -= remainingQty * bid.Price
			return totalAmount / (totalQty - remainingQty)
		}
	}
	return 0 // This indicates that there's not enough liquidity to fill the order
}

func updateAggDataOrderBook(aggData *AggregatedData, bids, asks []OrderLevel, mid float64) ([]CumulatedProfile, []CumulatedProfile) {
	// Compute total quantities
	BidTotalQty, AskTotalQty := 0.0, 0.0
	for _, bid := range bids {
		BidTotalQty += bid.Quantity
	}
	for _, ask := range asks {
		AskTotalQty += ask.Quantity
	}
	aggData.BidTotalQty = 0.0
	cumBidQty := 0.0
	for _, bid := range bids {
		cumBidQty += bid.Quantity
		if cumBidQty < QUANTILE*BidTotalQty {
			aggData.BidTotalQty = cumBidQty
		} else {
			break
		}
	}
	aggData.AskTotalQty = 0.0
	cumAskQty := 0.0
	for _, Ask := range asks {
		cumAskQty += Ask.Quantity
		if cumAskQty < QUANTILE*AskTotalQty {
			aggData.AskTotalQty = cumAskQty
		} else {
			break
		}
	}
	// Compute cumulated profiles
	cumulatedBidLiquidity := 0.0
	BidMaxDelta := 0.0
	var cumBids []CumulatedProfile
	for _, bid := range bids {
		delta := mid - bid.Price
		cumulatedBidLiquidity += bid.Quantity
		normalizedLiquidity := cumulatedBidLiquidity / aggData.BidTotalQty
		if cumulatedBidLiquidity <= aggData.BidTotalQty {
			BidMaxDelta = delta
			cumBids = append(cumBids, CumulatedProfile{delta, normalizedLiquidity})
		} else {
			break
		}
	}
	cumulatedAskLiquidity := 0.0
	AskMaxDelta := 0.0
	var cumAsks []CumulatedProfile
	for _, ask := range asks {
		delta := ask.Price - mid
		cumulatedAskLiquidity += ask.Quantity
		normalizedLiquidity := cumulatedAskLiquidity / aggData.AskTotalQty
		if cumulatedAskLiquidity <= aggData.AskTotalQty {
			AskMaxDelta = delta
			cumAsks = append(cumAsks, CumulatedProfile{delta, normalizedLiquidity})
		} else {
			break
		}
	}
	// Normalize deltas
	for i := range cumBids {
		cumBids[i][0] /= BidMaxDelta
	}
	for i := range cumAsks {
		cumAsks[i][0] /= AskMaxDelta
	}
	aggData.BidMaxDelta = BidMaxDelta
	aggData.AskMaxDelta = AskMaxDelta
	if aggData.TotalNumber != 0 {
		aggData.OrderSize = aggData.TotalVolume / float64(aggData.TotalNumber)
		aggData.OrderAmount = aggData.TotalAmount / float64(aggData.TotalNumber)
	}
	if aggData.BidTotalNumber != 0 {
		aggData.BidOrderSize = aggData.BidTotalVolume / float64(aggData.BidTotalNumber)
		aggData.BidOrderAmount = aggData.BidTotalAmount / float64(aggData.BidTotalNumber)
	}
	if aggData.AskTotalNumber != 0 {
		aggData.AskOrderSize = aggData.AskTotalVolume / float64(aggData.AskTotalNumber)
		aggData.AskOrderAmount = aggData.AskTotalAmount / float64(aggData.AskTotalNumber)
	}
	if aggData.AskTotalAmount == 0.0 {
		aggData.AskTotalAmount = aggData.TotalAmount
		aggData.AskTotalVolume = aggData.TotalVolume
		aggData.AskTotalNumber = aggData.TotalNumber
	}
	if aggData.BidTotalAmount == 0.0 {
		aggData.BidTotalAmount = aggData.TotalAmount
		aggData.BidTotalVolume = aggData.TotalVolume
		aggData.BidTotalNumber = aggData.TotalNumber
	}
	// Compute spread based on average execution prices
	buyPrice := computeBuyPrice(asks, aggData.AskOrderSize)
	sellPrice := computeSellPrice(bids, aggData.BidOrderSize)
	aggData.BuyPrice = buyPrice
	aggData.SellPrice = sellPrice
	aggData.Spread = 2.0 * (buyPrice - sellPrice) / mid * BIPS
	return cumBids, cumAsks
}

func updateAccumulatedData(accumulatedData *AccumulatedData, aggData *AggregatedData, cumBids, cumAsks []CumulatedProfile) {
	accumulatedData.OrderSize += aggData.OrderSize
	accumulatedData.OrderAmount += aggData.OrderAmount
	accumulatedData.BidOrderSize += aggData.BidOrderSize
	accumulatedData.BidOrderAmount += aggData.BidOrderAmount
	accumulatedData.BidTotalQty += aggData.BidTotalQty
	accumulatedData.BidMaxDelta += aggData.BidMaxDelta
	accumulatedData.AskOrderSize += aggData.AskOrderSize
	accumulatedData.AskOrderAmount += aggData.AskOrderAmount
	accumulatedData.AskTotalQty += aggData.AskTotalQty
	accumulatedData.AskMaxDelta += aggData.AskMaxDelta
	accumulatedData.Spread += aggData.Spread
	normalizedCumBids := interpolateCumulatedProfile(cumBids)
	normalizedCumAsks := interpolateCumulatedProfile(cumAsks)
	// Assuming cumBids and cumAsks have the same length
	for i := range normalizedCumBids {
		accumulatedData.CumBids[i][0] += normalizedCumBids[i][0] // accumulate the liquidity value
		accumulatedData.CumAsks[i][0] += normalizedCumAsks[i][0]
		accumulatedData.CumBids[i][1] += normalizedCumBids[i][1] // accumulate the liquidity value
		accumulatedData.CumAsks[i][1] += normalizedCumAsks[i][1]
	}
}

func printAggData(data *AggregatedData) {
	fmt.Println("------------ Aggregated Data ------------")
	fmt.Printf("Last Timestamp: %v\n", data.LastTimestamp)
	fmt.Printf("Total Amount: %f\n", data.TotalAmount)
	fmt.Printf("Total Number: %d\n", data.TotalNumber)
	fmt.Printf("Total Volume: %f\n", data.TotalVolume)
	fmt.Printf("BuyPrice: %f\n", data.BuyPrice)
	fmt.Printf("SellPrice: %f\n", data.SellPrice)
	fmt.Printf("Mid Price: %f\n", data.MidPrice)
	fmt.Printf("Spread: %f\n", data.Spread)
	fmt.Printf("Order Size: %f\n", data.OrderSize)
	fmt.Printf("Order Amount: %f\n", data.OrderAmount)
	fmt.Printf("Bid Total Amount: %f\n", data.BidTotalAmount)
	fmt.Printf("Bid Total Number: %d\n", data.BidTotalNumber)
	fmt.Printf("Bid Total Volume: %f\n", data.BidTotalVolume)
	fmt.Printf("Bid Order Size: %f\n", data.BidOrderSize)
	fmt.Printf("Bid Order Amount: %f\n", data.BidOrderAmount)
	fmt.Printf("Ask Total Amount: %f\n", data.AskTotalAmount)
	fmt.Printf("Ask Total Number: %d\n", data.AskTotalNumber)
	fmt.Printf("Ask Total Volume: %f\n", data.AskTotalVolume)
	fmt.Printf("Ask Order Size: %f\n", data.AskOrderSize)
	fmt.Printf("Ask Order Amount: %f\n", data.AskOrderAmount)
	fmt.Printf("Bid Total Quantity: %f\n", data.BidTotalQty)
	fmt.Printf("Ask Total Quantity: %f\n", data.AskTotalQty)
	fmt.Printf("Bid Max Delta: %f\n", data.BidMaxDelta)
	fmt.Printf("Ask Max Delta: %f\n", data.AskMaxDelta)
	fmt.Println("-----------------------------------------")
}

func printOrderBook(data *OrderBookResponse) {
	fmt.Println("------------ Order Book Data ------------")
	fmt.Printf("Last Update ID: %d\n", data.LastUpdateID)
	fmt.Println("Bids:")
	for i, bid := range data.Bids {
		if i >= ORDERBOOK_PRINT_LINE_NB {
			break
		}
		fmt.Printf("Price: %s, Quantity: %s\n", bid[0], bid[1])
	}
	fmt.Println("Asks:")
	for i, ask := range data.Asks {
		if i >= ORDERBOOK_PRINT_LINE_NB {
			break
		}
		fmt.Printf("Price: %s, Quantity: %s\n", ask[0], ask[1])
	}
	fmt.Println("-----------------------------------------")
}

func processTicker(aggData *AggregatedData, accumulatedData *AccumulatedData, totalTicks *int64) error {
	orderBook, err := fetchOrderBook()
	if err != nil {
		return fmt.Errorf("error fetching order book: %v", err)
	}
	bids, err := convertLevels(orderBook.Bids)
	if err != nil {
		return fmt.Errorf("error converting bids: %v", err)
	}
	asks, err := convertLevels(orderBook.Asks)
	if err != nil {
		return fmt.Errorf("error converting asks: %v", err)
	}
	// Compute mid
	mid := (bids[0].Price + asks[0].Price) / 2
	aggData.MidPrice = mid
	// Update aggData (OrderBook parts)
	cumBids, cumAsks := updateAggDataOrderBook(aggData, bids, asks, mid)
	// Update accumulatedData based on aggData
	updateAccumulatedData(accumulatedData, aggData, cumBids, cumAsks)
	*totalTicks += 1
	fmt.Println(">>> Processed tick:", *totalTicks)
	printAggData(aggData)
	printOrderBook(orderBook)
	fmt.Println("<<<")
	return nil
}

func normalizeAccumulatedData(data *AccumulatedData, totalTicks int64) {
	if totalTicks == 0 {
		return
	}
	coef := 1.0 / float64(totalTicks)
	data.OrderSize *= coef
	data.OrderAmount *= coef
	data.BidOrderSize *= coef
	data.BidOrderAmount *= coef
	data.BidTotalQty *= coef
	data.BidMaxDelta *= coef
	data.AskOrderSize *= coef
	data.AskOrderAmount *= coef
	data.AskTotalQty *= coef
	data.AskMaxDelta *= coef
	data.Spread *= coef
	for i := range data.CumBids {
		data.CumBids[i][0] *= coef
		data.CumAsks[i][0] *= coef
		data.CumBids[i][1] *= coef
		data.CumAsks[i][1] *= coef
	}
}

func printAccumulatedData(data *AccumulatedData) {
	fmt.Printf("Total Spread: %f\n", data.Spread)
	fmt.Println("---------- Accumulated Data ----------")
	fmt.Printf("Order Size: %f\n", data.OrderSize)
	fmt.Printf("Order Amount: %f\n", data.OrderAmount)
	fmt.Printf("Bid Order Size: %f\n", data.BidOrderSize)
	fmt.Printf("Bid Order Amount: %f\n", data.BidOrderAmount)
	fmt.Printf("Bid Total Quantity: %f\n", data.BidTotalQty)
	fmt.Printf("Bid Max Delta: %f\n", data.BidMaxDelta)
	fmt.Printf("Ask Order Size: %f\n", data.AskOrderSize)
	fmt.Printf("Ask Order Amount: %f\n", data.AskOrderAmount)
	fmt.Printf("Ask Total Quantity: %f\n", data.AskTotalQty)
	fmt.Printf("Ask Max Delta: %f\n", data.AskMaxDelta)
	fmt.Println("Cumulative Bids:")
	for i, bid := range data.CumBids {
		if i >= ORDERBOOK_PRINT_LINE_NB {
			break
		}
		fmt.Printf("Delta: %f, Liquidity: %f\n", bid[0], bid[1])
	}
	fmt.Println("Cumulative Asks:")
	for i, ask := range data.CumAsks {
		if i >= ORDERBOOK_PRINT_LINE_NB {
			break
		}
		fmt.Printf("Delta: %f, Liquidity: %f\n", ask[0], ask[1])
	}
	fmt.Println("-------------------------------------")
}

func processSignal(sig os.Signal, accumulatedData *AccumulatedData, totalTicks int64, processStartTime int64, json_path string, symbol string, throttle int) {
	fmt.Println("Received signal:", sig)
	processStopTime := time.Now().Unix()
	normalizeAccumulatedData(accumulatedData, totalTicks)
	// Save accumulated data
	data, err := json.MarshalIndent(accumulatedData, "", "  ")
	if err != nil {
		log.Printf("Error marshalling accumulated data: %v", err)
	}
	printAccumulatedData(accumulatedData)
	filename := fmt.Sprintf("%v/capture_symbol=%v_throttle=%v_start=%d_end=%d.json", json_path, symbol, throttle, processStartTime, processStopTime)
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		log.Printf("Error writing to file: %v", err)
	}
	fmt.Println("Shutting down gracefully...")
}

func main() {
	fmt.Println("symbol: ", symbol, "json_path: ", json_path, "throttle: ", throttle)
	trades := make(chan *TradeResponse)
	go handleTradeStream(trades)
	ticker := time.NewTicker(time.Duration(throttle) * time.Second)
	defer ticker.Stop()
	aggData := AggregatedData{}
	var accumulatedData AccumulatedData
	var processStartTime = time.Now().Unix()
	var totalTicks int64 = 0
	// Signal handling
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case trade := <-trades:
			processTrade(trade, &aggData)
		case <-ticker.C:
			processTicker(&aggData, &accumulatedData, &totalTicks)
			aggData = AggregatedData{}
		case sig := <-signals:
			processSignal(sig, &accumulatedData, totalTicks, processStartTime, json_path, symbol, throttle)
			return
		}
	}
}
