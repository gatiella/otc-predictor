# 🎯 OTC Predictor - Real-Time Trading Predictions

High-accuracy prediction system for OTC (Over-The-Counter) markets including Volatility Indices and Crash/Boom markets.

## ✨ Features

- **Real-time price data** from Deriv WebSocket API
- **Multiple strategies**: Mean reversion, momentum, pattern recognition
- **High win rate target**: Optimized for 60%+ accuracy
- **Live predictions**: WebSocket support for real-time updates
- **Performance tracking**: Automatic win/loss tracking and statistics
- **Web dashboard**: Beautiful UI for monitoring predictions
- **RESTful API**: Easy integration with trading platforms

## 📊 Supported Markets

- Volatility Indices: V10, V25, V50, V75, V100
- Crash Indices: Crash 300, 500, 1000
- Boom Indices: Boom 300, 500, 1000

## 🚀 Quick Start

### Prerequisites

- Go 1.21 or higher
- Internet connection for real-time data

### Installation

```bash
# Clone repository
git https://github.com/gatiella/otc-predictor.git
cd otc-predictor

# Install dependencies
go mod download

# Run the application
go run cmd/main.go
```

### First Run

1. Application starts collecting data (wait 30 seconds)
2. Open dashboard: `http://localhost:8080`
3. Select a market and get predictions!

## 📁 Project Structure

```
otc-predictor/
├── cmd/
│   └── main.go                 # Application entry point
├── internal/
│   ├── api/                    # REST API & WebSocket
│   ├── collector/              # Data collection from Deriv
│   ├── indicators/             # Technical indicators (RSI, EMA, BB)
│   ├── predictor/              # Prediction engine
│   ├── storage/                # In-memory data storage
│   ├── strategy/               # Trading strategies
│   └── tracker/                # Performance tracking
├── pkg/
│   └── types/                  # Data structures
├── web/
│   └── dashboard.html          # Web dashboard
├── config.yaml                 # Configuration
└── go.mod                      # Dependencies
```

## 🔧 Configuration

Edit `config.yaml` to customize:

```yaml
strategy:
  min_confidence: 0.65          # Minimum confidence to trade
  rsi_period: 14
  rsi_overbought: 75
  rsi_oversold: 25

risk:
  max_predictions_per_minute: 10
  min_ticks_required: 200
```

## 📡 API Endpoints

### Get Prediction
```bash
GET /api/predict/:market/:duration
Example: curl http://localhost:8080/api/predict/volatility_75_1s/60
```

### Get Statistics
```bash
GET /api/stats/:market
Example: curl http://localhost:8080/api/stats/volatility_75_1s
```

### WebSocket Stream
```javascript
ws://localhost:8080/api/stream/volatility_75_1s?duration=60
```

### All Endpoints
- `GET /api/health` - Health check
- `GET /api/markets` - List active markets
- `GET /api/predict/:market/:duration` - Get prediction
- `GET /api/predict/all/:duration` - All predictions
- `GET /api/stats` - All statistics
- `GET /api/stats/:market` - Market statistics
- `GET /api/results/:market` - Trade results
- `GET /api/performance` - Performance summary

## 🎯 How It Works

### 1. Data Collection
- Connects to Deriv WebSocket API
- Collects real-time price ticks
- Stores last 1000 ticks per market in memory

### 2. Technical Analysis
- **RSI**: Identifies overbought/oversold conditions
- **EMA**: Detects trends (9, 21, 50 periods)
- **Bollinger Bands**: Finds price extremes
- **Momentum**: Measures price velocity
- **Patterns**: Detects double tops/bottoms

### 3. Strategy Execution

**For Volatility Indices:**
- Mean reversion (strongest for OTC)
- Momentum following
- Bollinger Band squeeze
- RSI extremes

**For Crash/Boom:**
- Spike detection
- Between-spike trends
- Volatility analysis

### 4. Consensus Voting
- Multiple strategies vote
- Weighted by historical accuracy
- Only predicts if confidence > 65%
- Quality filters prevent bad trades

### 5. Result Tracking
- Monitors each prediction
- Checks outcome after duration
- Calculates win rate automatically
- Updates statistics in real-time

## 📈 Example Prediction

```json
{
  "market": "volatility_75_1s",
  "direction": "UP",
  "confidence": 0.68,
  "reason": "3/4 strategies agree UP: [MeanReversion(↑68%), Momentum(↑63%), RSI(↑62%)]",
  "current_price": 1456.32,
  "duration": 60,
  "indicators": {
    "rsi": 22.4,
    "ema_9": 1458.21,
    "bb_position": -0.85
  }
}
```

## 🎮 Using the Dashboard

1. **Select Market**: Choose from dropdown
2. **Set Duration**: 30s, 60s, 120s, etc.
3. **Get Prediction**: Click button or enable live mode
4. **Live Mode**: Real-time updates every 3 seconds
5. **View Stats**: See win rates and performance

## 📊 Performance Monitoring

The system automatically tracks:
- Total trades per market
- Win/loss ratio
- Win rate percentage
- Profit/loss (assuming $10 stakes)
- Current win streak
- Best win streak

View performance:
```bash
curl http://localhost:8080/api/performance
```

## ⚙️ Advanced Configuration

### Adjust Strategy Weights

Edit `internal/strategy/combined.go`:
```go
meanReversionWeight: 0.4    // Most reliable for OTC
momentumWeight: 0.3
patternWeight: 0.3
```

### Change Minimum Confidence

Edit `config.yaml`:
```yaml
strategy:
  min_confidence: 0.70  # Higher = fewer but better trades
```

### Rate Limiting

```yaml
risk:
  max_predictions_per_minute: 10  # Prevent overtrading
```

## 🔍 Troubleshooting

### No markets active
- Wait 30-60 seconds for data collection
- Check internet connection
- Verify Deriv API is accessible

### Low win rate
- Increase `min_confidence` in config
- Wait for more data (200+ ticks)
- Avoid high volatility periods

### Connection lost
- Auto-reconnects after 5 seconds
- Check Deriv API status
- Verify websocket URL in config

## 🛡️ Risk Disclaimer

This software is for educational purposes only. Trading financial instruments carries risk. Past performance does not guarantee future results. Always:

- Start with demo accounts
- Never risk more than you can afford to lose
- Understand that even high win rates can result in losses
- Use proper risk management

## 📝 License

MIT License - Use at your own risk

## 🤝 Contributing

Contributions welcome! Areas for improvement:
- Additional technical indicators
- Machine learning models
- Backtesting framework
- More markets support

## 📧 Support

For issues and questions:
- Open an issue on GitHub
- Check the logs in console
- Review API responses for errors

## 🎉 Enjoy Trading!

Remember: The goal is consistent profitability, not every single trade. The system is designed to skip uncertain trades to maintain high accuracy.

Good luck! 🚀