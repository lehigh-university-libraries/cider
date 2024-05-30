package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	ring "github.com/zealws/golang-ring"
)

const (
	bufferSize = 10000
	timeWindow = 10 * time.Minute
)

type CIDRRequestCount struct {
	Count      int
	LastUpdate time.Time
}

type ReverseProxy struct {
	Target       *url.URL
	Ips          []string
	OriginalHost []string
}

var (
	ipRequestCounts  = make(map[string]*CIDRRequestCount)
	ringBuffer       ring.Ring
	ringMux          sync.Mutex
	backendHost      string
	requestThreshold int
)

func main() {
	var err error
	backendHost = os.Getenv("BACKEND_HOST")
	if backendHost == "" {
		slog.Error("Need to know where to proxy successful requests to.")
		os.Exit(1)
	}

	threshold := os.Getenv("REQUEST_THRESHOLD")
	requestThreshold, err = strconv.Atoi(threshold)
	if err != nil {
		slog.Warn("Setting default threshold")
		requestThreshold = 100
	}

	ringBuffer.SetCapacity(bufferSize)

	http.HandleFunc("/", handleRequest)

	slog.Info("Server is running on :8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		slog.Error("Unable to start service")
		os.Exit(1)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		slog.Error("Unable to get IP from IP:port", "ip", ip)
		http.Error(w, "Invalid remote address", http.StatusInternalServerError)
		return
	}
	cidr, err := GetCIDR(ip)
	if err != nil {
		slog.Error("Unable to get CIDR from IP", "ip", ip)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
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

	i1, i2 := ReadUserIP(r)
	rp := &ReverseProxy{
		Target: &url.URL{
			Host:   backendHost,
			Scheme: "http",
		},
		Ips: []string{
			i1,
			i2,
		},
	}
	rp.ServeHTTP(w, r)
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

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(p.Target)
			pr.Out.Header["X-Forwarded-For"] = p.Ips
			pr.SetXForwarded()
		},
		// http.DefaultTransport with Timeout/KeepAlive upped from 30s to 120s
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   120 * time.Second,
				KeepAlive: 120 * time.Second,
			}).DialContext,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{
					Timeout:   120 * time.Second,
					KeepAlive: 120 * time.Second,
				}
				return tls.DialWithDialer(dialer, network, addr, nil)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	proxy.ServeHTTP(w, r)
}

func ReadUserIP(r *http.Request) (string, string) {
	realIP := r.Header.Get("X-Real-Ip")
	lastIP := r.RemoteAddr
	if realIP == "" {
		realIP = r.Header.Get("X-Forwarded-For")
	}
	return realIP, lastIP
}
