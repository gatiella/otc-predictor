package collector

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"otc-predictor/internal/storage"
	"otc-predictor/pkg/types"

	"github.com/gorilla/websocket"
)

// OTCCollector collects price data from Deriv WebSocket API
type OTCCollector struct {
	storage       *storage.MemoryStorage
	config        types.DataSourceConfig
	markets       []string
	connections   map[string]*websocket.Conn
	connMu        sync.RWMutex
	reconnect     chan string
	subscribed    map[string]bool
	subMu         sync.RWMutex
	stopChan      chan bool
	marketBatches [][]string
}

// DerivMessage represents Deriv API message
type DerivMessage struct {
	MsgType string                 `json:"msg_type"`
	Tick    *DerivTick             `json:"tick,omitempty"`
	Error   *DerivError            `json:"error,omitempty"`
	Echo    map[string]interface{} `json:"echo_req,omitempty"`
}

// DerivTick represents a price tick from Deriv
type DerivTick struct {
	Ask    float64 `json:"ask"`
	Bid    float64 `json:"bid"`
	Epoch  int64   `json:"epoch"`
	ID     string  `json:"id"`
	Pip    float64 `json:"pip_size"`
	Quote  float64 `json:"quote"`
	Symbol string  `json:"symbol"`
}

// DerivError represents an error from Deriv API
type DerivError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewOTCCollector creates a new OTC data collector
func NewOTCCollector(storage *storage.MemoryStorage, config types.DataSourceConfig, markets []string) *OTCCollector {
	// Split markets into batches of 3 to avoid policy violations
	batches := [][]string{}
	batchSize := 3

	for i := 0; i < len(markets); i += batchSize {
		end := i + batchSize
		if end > len(markets) {
			end = len(markets)
		}
		batches = append(batches, markets[i:end])
	}

	log.Printf("📊 Total markets: %d, Batches: %d", len(markets), len(batches))

	return &OTCCollector{
		storage:       storage,
		config:        config,
		markets:       markets,
		connections:   make(map[string]*websocket.Conn),
		reconnect:     make(chan string, 10),
		subscribed:    make(map[string]bool),
		stopChan:      make(chan bool),
		marketBatches: batches,
	}
}

// Start begins collecting data
func (c *OTCCollector) Start() error {
	log.Println("🚀 Starting multi-market data collector...")
	log.Printf("📈 Subscribing to %d markets in %d batches", len(c.markets), len(c.marketBatches))

	// Start connection manager for each batch
	for batchIdx, batch := range c.marketBatches {
		go c.connectionManager(batchIdx, batch)
		// Stagger connection starts
		time.Sleep(2 * time.Second)
	}

	return nil
}

// connectionManager handles connection and reconnection for a batch
func (c *OTCCollector) connectionManager(batchIdx int, markets []string) {
	connKey := fmt.Sprintf("batch_%d", batchIdx)
	backoffDelay := c.config.ReconnectDelay

	for {
		select {
		case <-c.stopChan:
			return
		default:
			if err := c.connectBatch(connKey, markets); err != nil {
				log.Printf("❌ Batch %d connection failed: %v", batchIdx, err)
				log.Printf("⏳ Retrying in %d seconds...", backoffDelay)
				time.Sleep(time.Duration(backoffDelay) * time.Second)

				// Exponential backoff up to 30 seconds
				backoffDelay *= 2
				if backoffDelay > 30 {
					backoffDelay = 30
				}
				continue
			}

			// Reset backoff on successful connection
			backoffDelay = c.config.ReconnectDelay

			// Subscribe to markets in this batch
			for _, market := range markets {
				if err := c.subscribe(connKey, market); err != nil {
					log.Printf("⚠️  Failed to subscribe to %s: %v", market, err)
				}
				time.Sleep(500 * time.Millisecond) // Stagger subscriptions
			}

			// Start reading messages
			c.readMessages(connKey)

			// Connection lost
			log.Printf("⚠️  Batch %d connection lost, reconnecting...", batchIdx)
			time.Sleep(time.Duration(c.config.ReconnectDelay) * time.Second)
		}
	}
}

// connectBatch establishes WebSocket connection for a batch
func (c *OTCCollector) connectBatch(connKey string, markets []string) error {
	conn, _, err := websocket.DefaultDialer.Dial(c.config.APIURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Deriv API: %w", err)
	}

	c.connMu.Lock()
	c.connections[connKey] = conn
	c.connMu.Unlock()

	log.Printf("✅ Connected to Deriv WebSocket API (%s) - %d markets", connKey, len(markets))

	// Start ping/pong to keep connection alive
	go c.keepAlive(connKey)

	return nil
}

// subscribe subscribes to a market's tick stream
func (c *OTCCollector) subscribe(connKey, market string) error {
	c.connMu.RLock()
	conn, exists := c.connections[connKey]
	c.connMu.RUnlock()

	if !exists || conn == nil {
		return fmt.Errorf("no connection for %s", connKey)
	}

	// Convert market name to Deriv symbol format
	symbol := c.marketToSymbol(market)

	subscribeMsg := map[string]interface{}{
		"ticks":     symbol,
		"subscribe": 1,
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", market, err)
	}

	c.subMu.Lock()
	c.subscribed[market] = true
	c.subMu.Unlock()

	log.Printf("📊 Subscribed to %s (%s)", market, symbol)

	return nil
}

// readMessages reads and processes incoming messages
func (c *OTCCollector) readMessages(connKey string) {
	defer func() {
		c.connMu.Lock()
		if conn, exists := c.connections[connKey]; exists && conn != nil {
			conn.Close()
		}
		delete(c.connections, connKey)
		c.connMu.Unlock()
	}()

	for {
		c.connMu.RLock()
		conn, exists := c.connections[connKey]
		c.connMu.RUnlock()

		if !exists || conn == nil {
			return
		}

		var msg DerivMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("⚠️  Read error (%s): %v", connKey, err)
			return
		}

		c.handleMessage(msg)
	}
}

// handleMessage processes a message from Deriv
func (c *OTCCollector) handleMessage(msg DerivMessage) {
	switch msg.MsgType {
	case "tick":
		if msg.Tick != nil {
			c.processTick(msg.Tick)
		}

	case "error":
		if msg.Error != nil {
			log.Printf("❌ API Error: %s - %s", msg.Error.Code, msg.Error.Message)
		}

	case "ping":
		// Response to ping is automatic in most cases
		return
	}
}

// processTick processes a price tick
func (c *OTCCollector) processTick(derivTick *DerivTick) {
	// Convert Deriv symbol back to our market name
	market := c.symbolToMarket(derivTick.Symbol)

	// Use mid price (average of bid and ask)
	price := (derivTick.Bid + derivTick.Ask) / 2

	tick := types.Tick{
		Market:    market,
		Price:     price,
		Timestamp: time.Unix(derivTick.Epoch, 0),
		Epoch:     derivTick.Epoch,
	}

	c.storage.AddTick(market, tick)

	// Log periodically (every 100th tick)
	count := c.storage.GetTickCount(market)
	if count%100 == 0 {
		log.Printf("📈 %s: %.5f (%d ticks collected)", market, price, count)
	}
}

// keepAlive sends periodic pings to keep connection alive
func (c *OTCCollector) keepAlive(connKey string) {
	ticker := time.NewTicker(time.Duration(c.config.PingInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.connMu.RLock()
			conn, exists := c.connections[connKey]
			c.connMu.RUnlock()

			if !exists || conn == nil {
				return
			}

			if err := conn.WriteJSON(map[string]interface{}{"ping": 1}); err != nil {
				log.Printf("⚠️  Ping failed (%s): %v", connKey, err)
				return
			}
		}
	}
}

// marketToSymbol converts our market name to Deriv symbol
// ✅ UPDATED: Added ALL 39 markets
func (c *OTCCollector) marketToSymbol(market string) string {
	symbolMap := map[string]string{
		// Synthetic indices (11)
		"volatility_10_1s":  "R_10",
		"volatility_25_1s":  "R_25",
		"volatility_50_1s":  "R_50",
		"volatility_75_1s":  "R_75",
		"volatility_100_1s": "R_100",
		"crash_300_1s":      "CRASH300",
		"crash_500_1s":      "CRASH500",
		"crash_1000_1s":     "CRASH1000",
		"boom_300_1s":       "BOOM300",
		"boom_500_1s":       "BOOM500",
		"boom_1000_1s":      "BOOM1000",

		// Forex pairs (28) - USD Majors
		"frxEURUSD": "frxEURUSD",
		"frxGBPUSD": "frxGBPUSD",
		"frxUSDJPY": "frxUSDJPY",
		"frxUSDCHF": "frxUSDCHF",
		"frxUSDCAD": "frxUSDCAD",
		"frxAUDUSD": "frxAUDUSD",
		"frxNZDUSD": "frxNZDUSD",
		"frxUSDNOK": "frxUSDNOK",

		// EUR Cross Pairs
		"frxEURGBP": "frxEURGBP",
		"frxEURJPY": "frxEURJPY",
		"frxEURCHF": "frxEURCHF",
		"frxEURCAD": "frxEURCAD",
		"frxEURAUD": "frxEURAUD",
		"frxEURNZD": "frxEURNZD",
		"frxEURNOK": "frxEURNOK",

		// GBP Cross Pairs
		"frxGBPJPY": "frxGBPJPY",
		"frxGBPCHF": "frxGBPCHF",
		"frxGBPCAD": "frxGBPCAD",
		"frxGBPAUD": "frxGBPAUD",
		"frxGBPNZD": "frxGBPNZD",
		"frxGBPNOK": "frxGBPNOK",

		// AUD Cross Pairs
		"frxAUDJPY": "frxAUDJPY",
		"frxAUDCAD": "frxAUDCAD",
		"frxAUDCHF": "frxAUDCHF",
		"frxAUDNZD": "frxAUDNZD",

		// Other Cross Pairs
		"frxNZDJPY": "frxNZDJPY",
		"frxCADJPY": "frxCADJPY",
		"frxCHFJPY": "frxCHFJPY",
	}

	if symbol, exists := symbolMap[market]; exists {
		return symbol
	}

	// If not found, return as-is
	return market
}

// symbolToMarket converts Deriv symbol to our market name
// ✅ UPDATED: Added ALL 39 markets
func (c *OTCCollector) symbolToMarket(symbol string) string {
	marketMap := map[string]string{
		// Synthetic indices (11)
		"R_10":      "volatility_10_1s",
		"R_25":      "volatility_25_1s",
		"R_50":      "volatility_50_1s",
		"R_75":      "volatility_75_1s",
		"R_100":     "volatility_100_1s",
		"CRASH300":  "crash_300_1s",
		"CRASH500":  "crash_500_1s",
		"CRASH1000": "crash_1000_1s",
		"BOOM300":   "boom_300_1s",
		"BOOM500":   "boom_500_1s",
		"BOOM1000":  "boom_1000_1s",

		// Forex pairs (28) - USD Majors
		"frxEURUSD": "frxEURUSD",
		"frxGBPUSD": "frxGBPUSD",
		"frxUSDJPY": "frxUSDJPY",
		"frxUSDCHF": "frxUSDCHF",
		"frxUSDCAD": "frxUSDCAD",
		"frxAUDUSD": "frxAUDUSD",
		"frxNZDUSD": "frxNZDUSD",
		"frxUSDNOK": "frxUSDNOK",

		// EUR Cross Pairs
		"frxEURGBP": "frxEURGBP",
		"frxEURJPY": "frxEURJPY",
		"frxEURCHF": "frxEURCHF",
		"frxEURCAD": "frxEURCAD",
		"frxEURAUD": "frxEURAUD",
		"frxEURNZD": "frxEURNZD",
		"frxEURNOK": "frxEURNOK",

		// GBP Cross Pairs
		"frxGBPJPY": "frxGBPJPY",
		"frxGBPCHF": "frxGBPCHF",
		"frxGBPCAD": "frxGBPCAD",
		"frxGBPAUD": "frxGBPAUD",
		"frxGBPNZD": "frxGBPNZD",
		"frxGBPNOK": "frxGBPNOK",

		// AUD Cross Pairs
		"frxAUDJPY": "frxAUDJPY",
		"frxAUDCAD": "frxAUDCAD",
		"frxAUDCHF": "frxAUDCHF",
		"frxAUDNZD": "frxAUDNZD",

		// Other Cross Pairs
		"frxNZDJPY": "frxNZDJPY",
		"frxCADJPY": "frxCADJPY",
		"frxCHFJPY": "frxCHFJPY",
	}

	if market, exists := marketMap[symbol]; exists {
		return market
	}

	// If not found, return as-is
	return symbol
}

// IsForexSymbol checks if a symbol is forex
func (c *OTCCollector) IsForexSymbol(symbol string) bool {
	return strings.HasPrefix(strings.ToLower(symbol), "frx")
}

// Stop stops the collector
func (c *OTCCollector) Stop() {
	close(c.stopChan)

	c.connMu.Lock()
	defer c.connMu.Unlock()

	for key, conn := range c.connections {
		if conn != nil {
			conn.Close()
		}
		delete(c.connections, key)
	}

	log.Println("🛑 Multi-market collector stopped")
}
