package bashrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const maxOutputBytes = 256 * 1024

type lease struct {
	ExecutionID   string `json:"executionId"`
	Configuration struct {
		Script         string `json:"script"`
		TimeoutSeconds int    `json:"timeoutSeconds"`
	} `json:"configuration"`
}

func Run(ctx context.Context, logger *slog.Logger, apiURL, token string) error {
	if token == "" {
		return errors.New("RUNNER_TOKEN is required")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runner/bash/lease", strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
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
		status, result, message := execute(ctx, job.Configuration.Script, job.Configuration.TimeoutSeconds)
		payload, _ := json.Marshal(map[string]any{"executionId": job.ExecutionID, "status": status, "result": result, "error": message})
		complete, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runner/bash/complete", bytes.NewReader(payload))
		complete.Header.Set("Authorization", "Bearer "+token)
		complete.Header.Set("Content-Type", "application/json")
		done, err := client.Do(complete)
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

func execute(parent context.Context, script string, timeoutSeconds int) (string, map[string]any, string) {
	if timeoutSeconds < 1 {
		timeoutSeconds = 30
	}
	if timeoutSeconds > 300 {
		timeoutSeconds = 300
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", "-s")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.Stdin = strings.NewReader(script)
	command.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/tmp"}
	var stdout, stderr limitedBuffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	result := map[string]any{"stdout": stdout.String(), "stderr": stderr.String(), "outputTruncated": stdout.truncated || stderr.truncated}
	if ctx.Err() == context.DeadlineExceeded {
		return "timed_out", result, "script timed out"
	}
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			result["exitCode"] = exitError.ExitCode()
		}
		return "failed", result, err.Error()
	}
	result["exitCode"] = 0
	return "succeeded", result, ""
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := maxOutputBytes - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return original, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(value)
	return original, nil
}

func (b *limitedBuffer) String() string { return b.buffer.String() }

func wait(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
}
