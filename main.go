package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	ring "github.com/zealws/golang-ring"
)

const (
	requestThreshold = 100
	bufferSize       = 10000
	timeWindow       = 10 * time.Minute
)

type CIDRRequestCount struct {
	Count      int
	LastUpdate time.Time
}

var (
	ipRequestCounts = make(map[string]*CIDRRequestCount)
	ringBuffer      ring.Ring
	ringMux         sync.Mutex
)

func main() {
	ringBuffer.SetCapacity(bufferSize)

	http.HandleFunc("/", handleRequest)

	slog.Info("Server is running on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		slog.Error("Unable to start service")
		os.Exit(1)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	cidr, err := GetCIDR(ip)
	if err != nil {
		slog.Error("Unable to get CIDR from IP", "ip", ip)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}

	ringMux.Lock()
	requestCount, exists := ipRequestCounts[cidr]
	if !exists {
		// Evict the oldest entry if the buffer is full
		if ringBuffer.ContentSize() >= bufferSize {
			oldest := ringBuffer.Dequeue()
			if oldest != nil {
				delete(ipRequestCounts, oldest.(string))
			}
		}
		requestCount = &CIDRRequestCount{}
		ipRequestCounts[cidr] = requestCount
		ringBuffer.Enqueue(cidr)
	}
	ringMux.Unlock()

	now := time.Now()
	if now.Sub(requestCount.LastUpdate) > timeWindow {
		requestCount.Count = 0
	}
	requestCount.Count++
	requestCount.LastUpdate = now

	if requestCount.Count >= requestThreshold {
		http.Redirect(w, r, "/captcha", http.StatusFound)
		return
	}

	// proxy
}

func GetCIDR(ip string) (string, error) {
	mask := "16"
	if strings.Contains(ip, ":") {
		mask = "64"
	}

	_, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%s", ip, mask))
	if err != nil {
		return "", err
	}

	return ipNet.String(), nil
}
