# Sample Rate Limiter

## Rate Limiter using Sliding Window Counter algorithm

pkg/server - CreateHttpServers
- Mimics pool of http servers waiting to receive client requests
- Client request can go to any server
- Each server uses the rate limiter package to enforce rate limit per client using unique client Id

pkg/limiter - Limiter
- Mimics rate limiter that can be used by all servers. Instead of using distributed KV store, it uses an in-memory
  data-structure global to all servers to enforce rate-limit per clientId

pkg/limiter - Rule
- Mimics cache of rules for rate limiting. For simplicity the cache is a single global variable that maintains the
  maxRequests per unit of time rule for all clients

pkg/tests/limiter_test.go - TestE2EServer
- Run this to see the working of the rate limiter. The test creates clients that talk to the pool of servers and get
  rate limited based on the rule set

```shell script
cd tests
go test -count=1 -run="TestE2ELimiter"
```

