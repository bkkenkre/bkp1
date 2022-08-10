package limiter

import (
	"sync"
	"time"
)

/////////////////////////////////////////////////////////////////
// RULES for rate limiting
/////////////////////////////////////////////////////////////////

// Rule encapsulates max requests per unit of time.
// Example: For 10 requests per second, maxRequests = 10, unit = time.Second
// Example: For 100 requests per minute, maxRequests = 100, unit = time.Minute
// TODO Allow rules to be created per service-request types
// TODO Allow rules to be deleted
// TODO Store rules in a separate RDBMS
type Rule struct {
	maxRequests int64
	unit        time.Duration
}

// Using this global as a cache
var rule *Rule

// AddRule Create a global rule to set max request per unit of time. See block comments above Rule type for more details
func AddRule(maxRequests int64, unit time.Duration) {
	rule = &Rule{
		maxRequests: maxRequests,
		unit:        unit,
	}
}

/////////////////////////////////////////////////////////////////
// RATE LIMITER based on Sliding Window Counter algorithm
/////////////////////////////////////////////////////////////////

// Limiter implements Sliding Window Counter algorithm.
// This package uses clientLimiterMap to track Sliding window counters per client. A new sliding window counter pair
// is created for each new client.
// TODO Memory management to evict least recently used clients to disk if memory thresholds are exceeded
type Limiter struct {
	// Start of the previous window
	prevWindow time.Time
	// numRequests seen in the previous window
	prevCounter int64
	// Start of the current window
	currWindow time.Time
	// numRequests seen so far in the current window
	currCounter int64
	// mutex to synchronize access to per client limiter
	lock sync.Mutex
}

var clientLimiterMap sync.Map

// AllowRequest Returns true if the client's request can be processed, otherwise false if it is rate limited
// Also returns the duration left for the current window to expire. The duration will be -1 if no rule has been
// configured
func AllowRequest(clientId string) (bool, time.Duration) {
	// Allow by default to protect from limiter errors making server unavailable
	if rule == nil {
		return true, time.Duration(-1)
	}

	// Check if limiter counters exists for the given client
	var l *Limiter
	if val, ok := clientLimiterMap.Load(clientId); ok {
		l = val.(*Limiter)
	} else {
		l = &Limiter{}
		clientLimiterMap.Store(clientId, l)
	}
	return l.Allow()
}

func (l *Limiter) Allow() (bool, time.Duration) {
	now := time.Now()
	newCurrWindow := now.Truncate(rule.unit)
	newPrevWindow := newCurrWindow.Add(-rule.unit) // time.Sub() says use time.Add(-d) for t-d
	newCurrCounter := int64(0)
	newPrevCounter := int64(0)

	func() {
		// Instead of using new kv pairs for the new windows, we will reuse the existing window counters for which we
		// need to lock the following. Lock can be avoided by storing new kv pairs and a separate go-routine to clean
		// up older windows
		l.lock.Lock()
		defer l.lock.Unlock()

		if newCurrWindow == l.currWindow {
			// The current window is still active, increament the corresponding counter
			l.currCounter++
		} else {
			if newPrevWindow == l.currWindow {
				// We moved one window. Make the existing current window as the previous window
				l.prevWindow = l.currWindow
				l.prevCounter = l.currCounter
			} else {
				// We have moved many windows ahead. Reset the previous window
				l.prevWindow = time.Time{}
				l.prevCounter = 0
			}
			// Start the new current window
			l.currWindow = newCurrWindow
			l.currCounter = 0
		}

		newCurrWindow = l.currWindow
		newPrevWindow = l.prevWindow
		newCurrCounter = l.currCounter
		newPrevCounter = l.prevCounter
	}()

	prevWindowOverlap := rule.unit - now.Sub(newCurrWindow)
	prevWindowWeightedCounter := int64(float64(newPrevCounter) * (float64(prevWindowOverlap) / float64(rule.unit)))
	activeNumRequests := prevWindowWeightedCounter + newCurrCounter
	return activeNumRequests < rule.maxRequests, prevWindowOverlap
}

// ResetLimiter Clear all limiters
func ResetLimiter() {
	clientLimiterMap = sync.Map{}
}
