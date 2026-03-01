package agent

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	agentcontext "devopsctl/internal/agent/context"
	"devopsctl/internal/proto"
)

// handleHealthCheck performs a TCP or HTTP health check against a remote endpoint.
//
// Inputs (TCP mode — either host+port or addr required):
//   - host     (string) — hostname or IP
//   - port     (int|string) — port number
//   - addr     (string) — shorthand "host:port"
//
// Inputs (HTTP mode — when url is provided, takes precedence):
//   - url      (string) — full URL to GET
//   - expected_status (int, default 200) — expected HTTP status code
//
// Common inputs:
//   - timeout  (string, default "5s") — deadline for the check e.g. "10s"
//   - retries  (int, default 0) — number of additional attempts before failing
func handleHealthCheck(_ *agentcontext.Executor, inputs map[string]any) proto.Result {
	// Parse timeout.
	timeout := 5 * time.Second
	if ts, ok := inputs["timeout"].(string); ok && ts != "" {
		if d, err := time.ParseDuration(ts); err == nil {
			timeout = d
		}
	}

	// Parse retries.
	retries := 0
	if rv, ok := inputs["retries"]; ok {
		switch v := rv.(type) {
		case int:
			retries = v
		case float64:
			retries = int(v)
		case string:
			n, _ := strconv.Atoi(v)
			retries = n
		}
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		lastErr = doHealthCheck(inputs, timeout)
		if lastErr == nil {
			mode := checkMode(inputs)
			return proto.Result{
				Status:  "success",
				Message: fmt.Sprintf("health.check: %s is healthy (attempt %d/%d)", mode, attempt+1, retries+1),
				Stdout:  fmt.Sprintf("status=healthy attempts=%d", attempt+1),
			}
		}
	}

	return proto.Result{
		Status:  "failed",
		Message: fmt.Sprintf("health.check: %v (after %d attempt(s))", lastErr, retries+1),
		Stderr:  lastErr.Error(),
	}
}

// doHealthCheck performs a single health check attempt (HTTP or TCP).
func doHealthCheck(inputs map[string]any, timeout time.Duration) error {
	// HTTP mode.
	if urlStr, ok := inputs["url"].(string); ok && urlStr != "" {
		return checkHTTP(urlStr, inputs, timeout)
	}

	// TCP mode.
	addr := resolveAddr(inputs)
	if addr == "" {
		return fmt.Errorf("health.check: provide 'url' (HTTP) or 'host'+'port' / 'addr' (TCP)")
	}
	return checkTCP(addr, timeout)
}

// checkHTTP performs an HTTP GET and validates the status code.
func checkHTTP(url string, inputs map[string]any, timeout time.Duration) error {
	expectedStatus := 200
	if es, ok := inputs["expected_status"]; ok {
		switch v := es.(type) {
		case int:
			expectedStatus = v
		case float64:
			expectedStatus = int(v)
		case string:
			n, _ := strconv.Atoi(v)
			if n > 0 {
				expectedStatus = n
			}
		}
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("http GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("http GET %s: got status %d, want %d", url, resp.StatusCode, expectedStatus)
	}
	return nil
}

// checkTCP attempts a TCP connection to addr and closes it immediately.
func checkTCP(addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("tcp connect %s: %w", addr, err)
	}
	conn.Close()
	return nil
}

// resolveAddr builds a "host:port" address from inputs.
func resolveAddr(inputs map[string]any) string {
	if addr, ok := inputs["addr"].(string); ok && addr != "" {
		return addr
	}
	host, _ := inputs["host"].(string)
	if host == "" {
		return ""
	}
	port := ""
	switch v := inputs["port"].(type) {
	case string:
		port = v
	case int:
		port = strconv.Itoa(v)
	case float64:
		port = strconv.Itoa(int(v))
	}
	if port == "" {
		return ""
	}
	return net.JoinHostPort(host, port)
}

// checkMode returns a human-readable mode identifier for result messages.
func checkMode(inputs map[string]any) string {
	if url, ok := inputs["url"].(string); ok && url != "" {
		return "HTTP " + url
	}
	return "TCP " + resolveAddr(inputs)
}
