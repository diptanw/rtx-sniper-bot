package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

// RotatingTransport implements http.RoundTripper. It cycles through a list
// of HTTP proxies, and if all fail, falls back to a default transport.
type RotatingTransport struct {
	proxies  []*url.URL
	fallback http.RoundTripper
	index    uint32
}

// NewRotatingTransport creates a new RotatingTransport.
// The `proxies` slice can be addresses of HTTP proxies.
// For example: []string{"http://127.0.0.1:8080", "http://127.0.0.1:8081"}.
func NewRotatingTransport(proxyAddrs []string) (*RotatingTransport, error) {
	proxies := []*url.URL{
		nil, // Nil proxy is default (fallback) Transport.
	}

	for _, proxyAddr := range proxyAddrs {
		proxyURL, err := url.Parse(proxyAddr)
		if err != nil {
			return nil, fmt.Errorf("parsing proxy URL: %w", err)
		}

		proxies = append(proxies, proxyURL)
	}

	return &RotatingTransport{
		proxies:  proxies,
		fallback: http.DefaultTransport,
	}, nil
}

// RoundTrip implements the RoundTrip method of http.RoundTripper.
func (rt *RotatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(rt.proxies) == 0 {
		return rt.fallback.RoundTrip(req)
	}

	numProxies := len(rt.proxies)
	startIndex := atomic.AddUint32(&rt.index, 1) - 1

	for i := range make([]int, numProxies) {
		idx := (int(startIndex) + i) % numProxies

		if rt.proxies[idx] == nil {
			break
		}

		transport := http.Transport{
			Proxy: http.ProxyURL(rt.proxies[idx]),
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}

		resp, err := transport.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
	}

	// If all proxies failed, fallback to default transport.
	return rt.fallback.RoundTrip(req)
}
