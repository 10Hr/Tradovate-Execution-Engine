package marketdata

import (
	"log"
	"time"
)

// HistoricalDataClient handles historical market data requests
type HistoricalDataClient struct {
	client WebSocketSender
}

// NewHistoricalDataClient creates a new historical data client
func NewHistoricalDataClient(client WebSocketSender) *HistoricalDataClient {
	return &HistoricalDataClient{
		client: client,
	}
}

// GetTickData retrieves historical tick data for a date range
func (h *HistoricalDataClient) GetTickData(symbol interface{}, startTime, endTime time.Time) error {
	params := HistoricalDataParams{
		Symbol: symbol,
		ChartDescription: ChartDesc{
			UnderlyingType: "Tick",
		},
		TimeRange: TimeRange{
			AsFarAsTimestamp: startTime.Format(time.RFC3339),
			ClosestTimestamp: endTime.Format(time.RFC3339),
		},
	}

	log.Printf("Requesting tick data from %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	return h.client.Send("md/getchart", params)
}

// GetBarData retrieves historical bar data for a date range
func (h *HistoricalDataClient) GetBarData(symbol interface{}, startTime, endTime time.Time, barSize int, barUnit string) error {
	params := HistoricalDataParams{
		Symbol: symbol,
		ChartDescription: ChartDesc{
			UnderlyingType:  "MinuteBar",
			ElementSize:     barSize,
			ElementSizeUnit: barUnit,
		},
		TimeRange: TimeRange{
			AsFarAsTimestamp: startTime.Format(time.RFC3339),
			ClosestTimestamp: endTime.Format(time.RFC3339),
		},
	}

	log.Printf("Requesting bar data from %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	return h.client.Send("md/getchart", params)
}

// GetDailyBarData retrieves historical daily bar data for a date range
func (h *HistoricalDataClient) GetDailyBarData(symbol interface{}, startTime, endTime time.Time) error {
	params := HistoricalDataParams{
		Symbol: symbol,
		ChartDescription: ChartDesc{
			UnderlyingType: "DailyBar",
		},
		TimeRange: TimeRange{
			AsFarAsTimestamp: startTime.Format(time.RFC3339),
			ClosestTimestamp: endTime.Format(time.RFC3339),
		},
	}

	log.Printf("Requesting daily bar data from %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	return h.client.Send("md/getchart", params)
}

// GetCustomChart retrieves historical data with custom chart parameters
func (h *HistoricalDataClient) GetCustomChart(symbol interface{}, chartDesc ChartDesc, timeRange TimeRange) error {
	params := HistoricalDataParams{
		Symbol:           symbol,
		ChartDescription: chartDesc,
		TimeRange:        timeRange,
	}

	log.Printf("Requesting custom chart data")
	return h.client.Send("md/getchart", params)
}
