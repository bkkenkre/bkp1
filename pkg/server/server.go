package server

import (
	"bkp1/pkg/limiter"
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

const ServerPort = 8090
const ClientId = "clientId"

var handlers []func(w http.ResponseWriter, req *http.Request)
var m *http.ServeMux

type clientMetric struct {
	NumAccepted int
	NumRejected int
}

var clientMetricMap sync.Map

// Create 'numServers' HTTP servers
func CreateHttpServers(numServers int) {
	m = http.NewServeMux()
	s := http.Server{Addr: fmt.Sprintf(":%v", ServerPort), Handler: m}
	m.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
		go func() {
			if err := s.Shutdown(context.Background()); err != nil {
				log.Fatal(err)
			}
		}()
	})

	for ii := 0; ii < numServers; ii++ {
		jj := ii
		fn := func(w http.ResponseWriter, req *http.Request) {
			clientId := req.Header.Get(ClientId)
			allowRequest, timeLeft := limiter.AllowRequest(clientId)
			if allowRequest {
				now := time.Now()
				reportMetric(clientId, true)
				_, _ = fmt.Fprintf(w, "[C-%v -> S-%v] ALLOWED at %v", clientId, jj, now.String())
			} else {
				reportMetric(clientId, false)
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%v", timeLeft))
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
		}
		handlers = append(handlers, fn)
		m.HandleFunc(fmt.Sprintf("/endpoint-%v", jj), handlers[jj])
	}

	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
}

// Shutdown the HTTP server
func Shutdown() {
	_, err := http.Get(fmt.Sprintf("http://localhost:%v/shutdown", ServerPort))
	if err == nil {
		log.Print("Servers have been shutdown")
	} else {
		log.Printf("Server shutdown failed, err=%v", err)
	}
}

/////////////////////////////////////////////////////////////////
// Metrics
////////////////////////////////////////////////////////////////

// Used by server to track accepted and rejected requests per client
func reportMetric(id string, accepted bool) {
	val := clientMetric{}
	if v, ok := clientMetricMap.Load(id); ok {
		val = v.(clientMetric)
	}
	if accepted {
		val.NumAccepted++
	} else {
		val.NumRejected++
	}
	clientMetricMap.Store(id, val)
}

func PrintMetric() {
	totalAccepted := 0
	totalRejected := 0
	clientMetricMap.Range(func(k, v interface{}) bool {
		val := v.(clientMetric)
		fmt.Printf("C-%v => accepted:%v, rejected:%v\n", k, val.NumAccepted, val.NumRejected)
		totalAccepted += val.NumAccepted
		totalRejected += val.NumRejected
		return true
	})
	fmt.Printf("totalAccepted:%v + totalRejected:%v = totalRequests:%v\n", totalAccepted, totalRejected,
		totalAccepted+totalRejected)
}

func ResetMetric() {
	clientMetricMap = sync.Map{}
}
