package limiter_test

import (
	"bkp1/pkg/limiter"
	"bkp1/pkg/server"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"
)

// This test code can be used to validate rate limiting as follows:
// numServers = total number of servers serving the clients
// numClients = total number of unique clients talking to the servers
// 1. Create "numServers" to serve clients and add rate limiting rule using limiter.AddRule()
// 2. Each client will run a for-loop sending "numRequests" to the server pool
// 3. If a client request is rate-limited, it will get http.StatusTooManyRequests from the server and X-RateLimit-Reset
//    header which has the duration left in the current window used by the sliding window counter algorithm
// 4. Sliding window counter algorithm typically rejects requests if client continues to maintain the burst so the
//    client will delay sending its next request for that amount.
// 5. After each client has send numRequests, it prints the metrics collected by server per client which shows the
//    number of requests accepted and rejected by the server during the time interval of this test
func TestE2ELimiter(t *testing.T) {
	// total number of servers serving the clients
	numServers := 2

	// total number of unique clients talking to the servers
	numClients := 5

	// Create the servers
	server.CreateHttpServers(numServers)

	// Add rule to rate limit "maxRequests" per "unit" example: 2 requests per second per client
	limiter.AddRule(5, time.Second)

	var wg sync.WaitGroup

	// total number of requests from each client
	numRequests := 50

	start := time.Now()

	// Start the client requests for each client. The clientID will be set as a header in the request for the server
	// to use for rate limiting per client
	for ii := 0; ii < numClients; ii++ {
		wg.Add(1)
		clientId := ii
		go func() {
			defer wg.Done()
			for jj := 0; jj < numRequests; jj++ {
				serverEndpoint := rand.Intn(numServers)
				_, status, timeLeft := sendClientRequest(clientId, serverEndpoint)
				if status == http.StatusTooManyRequests && timeLeft > 0 {
					fmt.Printf("Too many requests, timeLeft=%v\n", timeLeft)
					time.Sleep(timeLeft)
				}
			}
		}()
	}
	wg.Wait()

	// Print metrics
	timeTaken := time.Since(start)
	server.PrintMetric()
	fmt.Printf("Time taken %v\n", timeTaken)
	server.Shutdown()
}

func sendClientRequest(clientId, serverEndpoint int) (error, int, time.Duration) {
	client := http.Client{}
	url := fmt.Sprintf("http://localhost:%v/endpoint-%v", server.ServerPort, serverEndpoint)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err, -1, -1
	}
	req.Header.Set(server.ClientId, fmt.Sprintf("%v", clientId))
	resp, err := client.Do(req)
	if err != nil {
		return err, -1, -1
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusTooManyRequests {
		timeLeftStr := resp.Header.Get("X-RateLimit-Reset")
		if len(timeLeftStr) > 0 {
			timeLeft, err := time.ParseDuration(timeLeftStr)
			if err != nil {
				return err, -1, -1
			}
			return nil, http.StatusTooManyRequests, timeLeft
		}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
	return nil, http.StatusOK, -1
}
