package httprunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type lease struct {
	ExecutionID   string `json:"executionId"`
	Configuration struct {
		Method          string `json:"method"`
		URL             string `json:"url"`
		Body            string `json:"body"`
		TimeoutSeconds  int    `json:"timeoutSeconds"`
		SecretValue     string `json:"secretValue"`
		SecretPlacement string `json:"secretPlacement"`
		SecretHeader    string `json:"secretHeader"`
	} `json:"configuration"`
}

func Run(ctx context.Context, logger *slog.Logger, apiURL, token, allowedHosts string) error {
	if token == "" {
		return errors.New("RUNNER_TOKEN is required")
	}
	allowlist := parseAllowlist(allowedHosts)
	if len(allowlist) == 0 {
		return errors.New("HTTP_RUNNER_ALLOWED_HOSTS is empty; runner remains disabled")
	}
	control := &http.Client{Timeout: 10 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runner/http/lease", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response, err := control.Do(req)
		if err != nil {
			logger.Error("lease failed", "error", err)
			wait(ctx)
			continue
		}
		if response.StatusCode == http.StatusNoContent {
			response.Body.Close()
			wait(ctx)
			continue
		}
		var job lease
		if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&job) != nil {
			response.Body.Close()
			logger.Error("invalid lease response", "status", response.StatusCode)
			wait(ctx)
			continue
		}
		response.Body.Close()
		jobCtx, cancelJob := context.WithCancel(ctx)
		cancelled := make(chan struct{})
		monitorDone := make(chan struct{})
		go monitorCancellation(jobCtx, control, apiURL, token, job.ExecutionID, cancelJob, cancelled, monitorDone)
		status, result, errorMessage := execute(jobCtx, job, allowlist)
		cancelJob()
		<-monitorDone
		select {
		case <-cancelled:
			logger.Info("execution cancelled by requester", "executionId", job.ExecutionID)
			continue
		default:
		}
		payload, _ := json.Marshal(map[string]any{"executionId": job.ExecutionID, "status": status, "result": result, "error": errorMessage})
		complete, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runner/http/complete", bytes.NewReader(payload))
		complete.Header.Set("Authorization", "Bearer "+token)
		complete.Header.Set("Content-Type", "application/json")
		done, err := control.Do(complete)
		if err != nil {
			logger.Error("completion report failed", "error", err)
			continue
		}
		_, _ = io.Copy(io.Discard, done.Body)
		done.Body.Close()
		if done.StatusCode != http.StatusOK {
			logger.Error("completion rejected", "status", done.StatusCode)
		}
	}
}

func monitorCancellation(ctx context.Context, client *http.Client, apiURL, token, executionID string, cancel context.CancelFunc, cancelled chan<- struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/api/v1/runner/http/executions/"+url.PathEscape(executionID), nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response, err := client.Do(request)
			if err != nil {
				continue
			}
			var state struct {
				Status string `json:"status"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&state)
			response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && state.Status == "cancelled" {
				close(cancelled)
				cancel()
				return
			}
		}
	}
}

func wait(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
}
func parseAllowlist(raw string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, item := range strings.Split(raw, ",") {
		host := strings.ToLower(strings.TrimSpace(item))
		if host != "" {
			result[host] = struct{}{}
		}
	}
	return result
}

func execute(parent context.Context, job lease, allowlist map[string]struct{}) (string, map[string]any, string) {
	timeout := job.Configuration.TimeoutSeconds
	if timeout < 1 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Second)
	defer cancel()
	parsed, err := validateDestination(ctx, job.Configuration.URL, allowlist)
	if err != nil {
		return "failed", map[string]any{}, err.Error()
	}
	method := strings.ToUpper(job.Configuration.Method)
	if method == "" {
		method = "POST"
	}
	if _, allowed := map[string]struct{}{http.MethodGet: {}, http.MethodPost: {}, http.MethodPut: {}, http.MethodPatch: {}, http.MethodDelete: {}}[method]; !allowed {
		return "failed", map[string]any{}, "HTTP method is not allowed"
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), strings.NewReader(job.Configuration.Body))
	if err != nil {
		return "failed", map[string]any{}, err.Error()
	}
	if job.Configuration.Body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	applySecretHeader(request, job.Configuration.SecretValue, job.Configuration.SecretPlacement, job.Configuration.SecretHeader)
	if job.Configuration.SecretValue != "" {
		defer func() { job.Configuration.SecretValue = "" }()
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, errors.New("destination resolves to a private or local address")
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		_, err := validateDestination(req.Context(), req.URL.String(), allowlist)
		return err
	}}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "timed_out", map[string]any{}, "request timed out"
		}
		return "failed", map[string]any{}, err.Error()
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "failed", map[string]any{}, err.Error()
	}
	responseBody := redactSecret(string(body), job.Configuration.SecretValue)
	result := map[string]any{"statusCode": response.StatusCode, "body": responseBody, "contentType": response.Header.Get("Content-Type")}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "failed", result, "HTTP status " + strconv.Itoa(response.StatusCode)
	}
	return "succeeded", result, ""
}

func applySecretHeader(request *http.Request, secret, placement, configuredHeader string) {
	if secret == "" {
		return
	}
	header := strings.TrimSpace(configuredHeader)
	if header == "" {
		header = "Authorization"
	}
	value := secret
	if placement == "" || placement == "bearer" {
		header = "Authorization"
		value = "Bearer " + secret
	}
	request.Header.Set(header, value)
}

func redactSecret(value, secret string) string {
	if secret == "" {
		return value
	}
	value = strings.ReplaceAll(value, "Bearer "+secret, "Bearer [REDACTED]")
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}

func validateDestination(ctx context.Context, raw string, allowlist map[string]struct{}) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("only absolute HTTP(S) URLs are allowed")
	}
	if parsed.User != nil {
		return nil, errors.New("credentials in URLs are not allowed")
	}
	host := strings.ToLower(parsed.Hostname())
	if _, allowed := allowlist[host]; !allowed {
		return nil, fmt.Errorf("destination host %q is not allowlisted", host)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, errors.New("private and local destinations are blocked")
		}
	}
	return parsed, nil
}

func publicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}
