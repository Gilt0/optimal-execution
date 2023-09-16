# optimal-execution

In this project, I started with a data extractor on binance. And I am happy to have contributors to implement aan execution system. The objective is mainly educational so have fun!

## Data Extraction
Once you've pulled the repo, create a folder data/json from the root of the project
```bash
mkdir -p data/json
```
To run the extraction try
```bash
go run cmd/dataextractor/dataextractor.go -symbol=BTCUSDT -json_path=data/json/ -throttle=5
```
If everything works, you should have prints on the screen like 
It provides an output like
```bash
symbol:  BTCUSDT json_path:  data/json/ throttle:  5
>>> Processed tick: 1
------------ Aggregated Data ------------
Last Timestamp: 1694867267234
Total Amount: 12831.961904
Total Number: 14
Total Volume: 0.483620
BuyPrice: 26533.150000
SellPrice: 26533.140000
Mid Price: 26533.145000
Spread: 0.007538
Order Size: 0.034544
Order Amount: 916.568707
Bid Total Amount: 263.208749
Bid Total Number: 1
Bid Total Volume: 0.009920
Bid Order Size: 0.009920
Bid Order Amount: 263.208749
Ask Total Amount: 12568.753155
Ask Total Number: 13
Ask Total Volume: 0.473700
Ask Order Size: 0.036438
Ask Order Amount: 966.827166
Bid Total Quantity: 371.889140
Ask Total Quantity: 254.197670
Bid Max Delta: 341.725000
Ask Max Delta: 316.845000
-----------------------------------------
------------ Order Book Data ------------
Last Update ID: 38853788103
Bids:
Price: 26533.14000000, Quantity: 2.17741000
Price: 26532.96000000, Quantity: 0.53350000
Price: 26532.95000000, Quantity: 0.04079000
Price: 26532.90000000, Quantity: 0.04085000
Price: 26532.85000000, Quantity: 0.03902000
Asks:
Price: 26533.15000000, Quantity: 5.53860000
Price: 26533.24000000, Quantity: 0.00200000
Price: 26533.63000000, Quantity: 0.10396000
Price: 26533.65000000, Quantity: 0.10396000
Price: 26533.67000000, Quantity: 0.10396000
-----------------------------------------
<<<
...
```
with prints on the screen every `n=throttle` seconds, as defined above. When you stop the extraction, a file is saved in data/json in the format `capture_symbol=BTCUSDT_throttle=5_start=1694867262_end=1694867272.json` where the timestamp will obviously be the ones of your start of the process and your end of it.

```javascript
{
  "Spread": 0.007543766220855038,
  "OrderSize": 0.006236945827489481,
  "OrderAmount": 165.35367702397087,
  "BidOrderSize": 0.005939920430722318,
  "BidOrderAmount": 157.47893286249285,
  "BidTotalQty": 395.6825525000046,
  "BidMaxDelta": 327.0700000000006,
  "AskOrderSize": 0.0049890625,
  "AskOrderAmount": 132.269875328125,
  "AskTotalQty": 235.42801750000615,
  "AskMaxDelta": 338.02500000000146,
  "CumBids": [
    [
      0,
      0
    ],
    [
      0.0001,
      0.04905527602005688
    ],
    [
      0.0002,
      0.04934029523169036
    ],
    ...
  ],
  "CumAsks": [
    [
      0,
      0
    ],
    [
      0.0001,
      0.01546911719944717
    ],
    ...
  ]
}
```
The final output is what will provide you with the necessary data to compute the optimal execution.

## Pricer

It should compute for an Implementation Shortfall, the appropriate strategy in time and its value. As a reminder, the problem to solve is some minimisation of

$Qm_0 - Q \bar s - \left( q_0m_0 + \int_{0}^{q_0} F^{-1}\left(\chi\right)d\chi \right) - \dots - \left( q_{N - 1}m_{N - 1} + \int_{0}^{q_{N - 1}} F^{-1}\left(\chi\right)d\chi \right)$
