package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
)

// Global state configuration
var (
	serialPort string
	baudRate   int
	httpPort   int

	dataLock                 sync.Mutex
	measurements             [][2]int
	curAverage               [2]float64
	lastMeasurementTimestamp int64
)

type WeatherPayload struct {
	Status           string `json:"status"`
	LastUpdatedEpoch int64  `json:"last_updated_epoch"`
	Latest           struct {
		Temperature *int `json:"temperature"`
		Humidity    *int `json:"humidity"`
	} `json:"latest"`
	RollingAverage2m struct {
		Temperature float64 `json:"temperature"`
		Humidity    float64 `json:"humidity"`
	} `json:"rolling_average_2min"`
}

func getAverages(list [][2]int, n int) [2]float64 {
	if len(list) == 0 {
		return [2]float64{0.0, 0.0}
	}

	start := len(list) - n
	if start < 0 {
		start = 0
	}
	subset := list[start:]

	sumTemp, sumHum := 0, 0
	for _, m := range subset {
		sumTemp += m[0]
		sumHum += m[1]
	}

	count := float64(len(subset))
	return [2]float64{
		float64(sumTemp) / count,
		float64(sumHum) / count,
	}
}

func picoSerialReader() {
	log.Printf("Connecting to Pico on %s...", serialPort)

	c := &serial.Config{Name: serialPort, Baud: baudRate}
	stream, err := serial.OpenPort(c)
	if err != nil {
		log.Fatalf("Error opening serial port: %v", err)
	}
	defer stream.Close()
	log.Printf("Connected to serial port %s.", serialPort)

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		cleanLine := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(cleanLine, "ERROR") || cleanLine == "" {
			continue
		}

		parts := strings.Split(cleanLine, ";")
		if len(parts) != 2 {
			continue
		}

		temp, err1 := strconv.Atoi(parts[0])
		hum, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}

		// Mutex guard resource block
		dataLock.Lock()
		measurements = append(measurements, [2]int{temp, hum})
		if len(measurements) > 1000 {
			measurements = measurements[len(measurements)-1000:]
		}
		curAverage = getAverages(measurements, 60)
		lastMeasurementTimestamp = time.Now().Unix()
		dataLock.Unlock()
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Serial reader stopped: %v", err)
	} else {
		log.Printf("Serial reader stopped.")
	}
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))
	acceptHeader := strings.ToLower(r.Header.Get("Accept"))

	dataLock.Lock()
	var latest [2]int
	hasData := len(measurements) > 0
	if hasData {
		latest = measurements[len(measurements)-1]
	}
	avgTemp, avgHum := curAverage[0], curAverage[1]
	lastSeen := lastMeasurementTimestamp
	dataLock.Unlock()

	status := "no_data"
	if hasData {
		status = "online"
	}

	payload := WeatherPayload{
		Status:           status,
		LastUpdatedEpoch: lastSeen,
	}
	if hasData {
		payload.Latest.Temperature = &latest[0]
		payload.Latest.Humidity = &latest[1]
	}
	payload.RollingAverage2m.Temperature = avgTemp
	payload.RollingAverage2m.Humidity = avgHum

	// API/Automated Request Filtering
	if strings.Contains(userAgent, "curl") || strings.Contains(acceptHeader, "json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		encoder.Encode(payload)
		return
	}

	// Browser Fallback Output Template Layout
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var tempStr, humStr, timeStr string = "N/A", "N/A", "Never"
	if hasData {
		tempStr = fmt.Sprintf("%d°C", *payload.Latest.Temperature)
		humStr = fmt.Sprintf("%d%%", *payload.Latest.Humidity)
		timeStr = time.Unix(lastSeen, 0).Format(time.ANSIC)
	}

	fmt.Fprintf(w, `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Pico Weather Station</title>
		<meta http-equiv="refresh" content="5">
		<style>
			body { font-family: sans-serif; margin: 40px; background: #f4f6f9; color: #333; }
			.card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); max-width: 500px; }
			h1 { color: #2c3e50; margin-top: 0; }
			.metric { font-size: 1.2em; margin: 10px 0; }
			.val { font-weight: bold; color: #2980b9; }
		</style>
	</head>
	<body>
		<div class="card">
			<h1>Pico Weather Station</h1>
			<div class="metric">Latest Temperature: <span class="val">%s</span></div>
			<div class="metric">Latest Humidity: <span class="val">%s</span></div>
			<hr>
			<div class="metric">2-Min Avg Temp: <span class="val">%.2f°C</span></div>
			<div class="metric">2-Min Avg Humidity: <span class="val">%.2f%%</span></div>
			<p style="font-size:0.8em; color:#7f8c8d;">Last updated: %s</p>
		</div>
	</body>
	</html>`, tempStr, humStr, avgTemp, avgHum, timeStr)
}

func main() {
	flag.StringVar(&serialPort, "serialport", "/dev/ttyACM0", "Path to serial device interface file description node")
	flag.IntVar(&baudRate, "rate", 115200, "Serial port bit transmission interface speed")
	flag.IntVar(&httpPort, "port", 8080, "Port targeting listening network traffic pipelines")
	flag.Parse()

	// Spin background loop routine task
	go picoSerialReader()

	http.HandleFunc("/", httpHandler)
	log.Printf("Server running on http://localhost:%d ...", httpPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil))
}
