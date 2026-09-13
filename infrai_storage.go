package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type InfraiClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewInfraiClient(baseURL, apiKey string, httpClient *http.Client) *InfraiClient {
	return &InfraiClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

type presignPutRequest struct {
	Op             string `json:"op"`
	ExpiresSeconds int    `json:"expires_seconds"`
	ContentType    string `json:"content_type"`
	MaxBytes       int64  `json:"max_bytes"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PresignedUpload struct {
	URL string `json:"url"`
}

func (c *InfraiClient) PresignPut(ctx context.Context, bucket, key, contentType string, maxBytes int64, idempotencyKey string) (PresignedUpload, error) {
	// infrai.storage.object.presign: bucket and key are escaped path segments.
	path := "/v1/storage/object/presign/" + url.PathEscape(bucket) + "/" + escapeObjectKey(key)
	body := presignPutRequest{
		Op:             "put",
		ExpiresSeconds: 600,
		ContentType:    contentType,
		MaxBytes:       maxBytes,
		IdempotencyKey: idempotencyKey,
	}
	var result PresignedUpload
	err := c.call(ctx, http.MethodPost, path, body, &result)
	return result, err
}

func escapeObjectKey(key string) string {
	parts := strings.Split(key, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func (c *InfraiClient) call(ctx context.Context, method, path string, body any, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		response, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response envelope: %w", err)
		}
		if !env.OK {
			apiErr := envelopeError(env.Error, response.StatusCode)
			if response.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				delay := retryDelay(response.Header.Get("Retry-After"), attempt)
				if err := c.sleep(ctx, delay); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}
		if response.StatusCode >= 500 {
			return &InfraiError{Message: http.StatusText(response.StatusCode), HTTPStatus: response.StatusCode}
		}
		if result != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, result); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("request retry budget exhausted")
}

func envelopeError(details *apiError, status int) *InfraiError {
	if details == nil {
		return &InfraiError{Message: http.StatusText(status), HTTPStatus: status}
	}
	message := details.Message
	if message == "" {
		message = details.Hint
	}
	return &InfraiError{Code: details.Code, Message: message, HTTPStatus: status}
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
