package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeeeeeeM/laya-decision-web/internal/decision"
)

const (
	Endpoint = "https://jev.bocha.cn/v1/systemone"
	Model    = "bocha-jev-v1"
)

type Client struct {
	APIKey   string
	Endpoint string
	Timeout  time.Duration
	MaxQPS   float64
	HTTP     *http.Client

	mu       sync.Mutex
	lastCall time.Time
}

func New(apiKey string, timeout time.Duration, maxQPS float64) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if maxQPS <= 0 {
		maxQPS = 2
	}
	return &Client{
		APIKey:   apiKey,
		Endpoint: Endpoint,
		Timeout:  timeout,
		MaxQPS:   maxQPS,
		HTTP:     &http.Client{Timeout: timeout},
	}
}

func (c *Client) Name() string { return decision.ProviderBochaJev }

func (c *Client) Available() bool { return strings.TrimSpace(c.APIKey) != "" }

type RateLimitedError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider rate limited"
}

func (e *RateLimitedError) GetRetryAfter() time.Duration { return e.RetryAfter }

type AuthError struct{ Message string }

func (e *AuthError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider auth failed"
}

type UnavailableError struct{ Message string }

func (e *UnavailableError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider unavailable"
}

func (c *Client) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	if !c.Available() {
		return decision.Response{}, &UnavailableError{Message: "Bocha Jev API key is not configured"}
	}
	state, err := decision.StateAsAny(req.State)
	if err != nil {
		return decision.Response{}, err
	}
	questions, err := decision.NormalizeQuestions(req.Questions)
	if err != nil {
		return decision.Response{}, err
	}
	wireQuestions := make(map[string]any, len(questions))
	for k, q := range questions {
		item := map[string]any{
			"type":         string(q.Type),
			"instructions": q.Instructions,
		}
		switch q.Type {
		case decision.TypeChoice:
			item["criteria"] = q.CriteriaMap
		case decision.TypeScore:
			item["criteria"] = q.CriteriaList
		}
		wireQuestions[k] = item
	}
	payload := map[string]any{
		"model":     Model,
		"state":     state,
		"questions": wireQuestions,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return decision.Response{}, err
	}
	if len(body) > 256*1024 {
		return decision.Response{}, fmt.Errorf("request exceeds 256 KiB limit")
	}

	if err := c.waitQPS(ctx); err != nil {
		return decision.Response{}, err
	}

	start := time.Now()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
		if err != nil {
			return decision.Response{}, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
		httpReq.Header.Set("Accept", "application/json")

		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			lastErr = err
			if attempt < 2 {
				if sleepErr := sleepCtx(ctx, time.Duration(0.5*math.Pow(2, float64(attempt))*float64(time.Second))); sleepErr != nil {
					return decision.Response{}, sleepErr
				}
				continue
			}
			return decision.Response{}, &UnavailableError{Message: "could not reach Bocha Jev API"}
		}
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return decision.Response{}, readErr
		}

		switch resp.StatusCode {
		case http.StatusOK:
			var parsed struct {
				Answers map[string]json.RawMessage `json:"answers"`
				Usage   decision.Usage             `json:"usage"`
			}
			if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.Answers == nil {
				return decision.Response{}, fmt.Errorf("Bocha Jev returned an invalid decision response")
			}
			return decision.Response{
				Provider: decision.ProviderBochaJev,
				Model:    Model,
				Answers:  parsed.Answers,
				Usage:    parsed.Usage,
				Timing: decision.Timing{
					ProviderMS: float64(time.Since(start).Microseconds()) / 1000,
				},
			}, nil
		case http.StatusUnauthorized:
			return decision.Response{}, &AuthError{Message: "Bocha Jev rejected the API key"}
		case http.StatusTooManyRequests, http.StatusServiceUnavailable, 529:
			delay := retryAfter(resp.Header.Get("Retry-After"), attempt)
			if delay > 10*time.Second || attempt == 2 {
				return decision.Response{}, &RateLimitedError{
					RetryAfter: delay,
					Message:    fmt.Sprintf("Bocha Jev rate limited (HTTP %d)", resp.StatusCode),
				}
			}
			if err := sleepCtx(ctx, delay); err != nil {
				return decision.Response{}, err
			}
			continue
		default:
			return decision.Response{}, fmt.Errorf("Bocha Jev request failed (HTTP %d)", resp.StatusCode)
		}
	}
	if lastErr != nil {
		return decision.Response{}, lastErr
	}
	return decision.Response{}, &UnavailableError{Message: "Bocha Jev request failed after retries"}
}

func (c *Client) waitQPS(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	minGap := time.Duration(float64(time.Second) / c.MaxQPS)
	wait := minGap - time.Since(c.lastCall)
	if wait > 0 {
		c.mu.Unlock()
		err := sleepCtx(ctx, wait)
		c.mu.Lock()
		if err != nil {
			return err
		}
	}
	c.lastCall = time.Now()
	return nil
}

func retryAfter(value string, attempt int) time.Duration {
	if value != "" {
		if secs, err := strconv.ParseFloat(value, 64); err == nil {
			return time.Duration(secs * float64(time.Second))
		}
		if t, err := http.ParseTime(value); err == nil {
			d := time.Until(t)
			if d < 0 {
				d = 0
			}
			return d
		}
	}
	return time.Duration(0.5*math.Pow(2, float64(attempt)) * float64(time.Second))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
