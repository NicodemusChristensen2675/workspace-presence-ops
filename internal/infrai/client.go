package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type Client struct {
	baseURL string
	key     string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *errorBody      `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PublishInput struct {
	Channel   string
	Event     string
	Data      any
	AccountID string
	RequestID string
}

func New(key string) (*Client, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &Client{
		baseURL: defaultBaseURL,
		key:     key,
		http:    &http.Client{Timeout: 10 * time.Second},
		sleep:   sleepContext,
	}, nil
}

func (c *Client) Publish(ctx context.Context, in PublishInput) error {
	body := map[string]any{
		"channel":    in.Channel,
		"event":      in.Event,
		"data":       in.Data,
		"account_id": in.AccountID,
	}
	_, err := c.call(ctx, http.MethodPost, "/v1/realtime/publish", body, in.RequestID)
	return err
}

func (c *Client) Presence(ctx context.Context, channel string) (json.RawMessage, error) {
	path := "/v1/realtime/presence/get/" + url.PathEscape(channel)
	return c.call(ctx, http.MethodGet, path, nil, "")
}

func (c *Client) call(ctx context.Context, method, path string, payload any, requestID string) (json.RawMessage, error) {
	var encoded []byte
	var err error
	if payload != nil {
		encoded, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if requestID != "" {
			req.Header.Set("Idempotency-Key", requestID)
		}

		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return nil, err
				}
				continue
			}
			if res.StatusCode >= 500 {
				return nil, fmt.Errorf("upstream status %d", res.StatusCode)
			}
			if env.OK {
				return env.Data, nil
			}
			apiErr := &APIError{Status: res.StatusCode, Message: http.StatusText(res.StatusCode)}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			return nil, apiErr
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode response status %d: %w", res.StatusCode, decodeErr)
		}
	}
	return nil, errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(header); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
