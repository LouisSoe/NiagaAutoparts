package ai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// packageHTTPClient is reused across calls to take advantage of connection pooling.
var packageHTTPClient = &http.Client{}

// doHTTPPost performs a POST request with the given JSON body and returns the response bytes.
func doHTTPPost(ctx context.Context, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := packageHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http error %d: %s", resp.StatusCode, string(respBytes))
	}
	return respBytes, nil
}