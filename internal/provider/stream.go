package provider

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// errorf builds a consistent upstream-error message from a raw response body.
func errorf(name string, status int, raw []byte) error {
	return fmt.Errorf("%s: upstream error (status %d): %s", name, status, truncate(raw))
}

// readSSE reads a Server-Sent Events stream, invoking onData for each "data:"
// payload. It returns when the stream ends, "[DONE]" is seen, onData asks to
// stop, or onData returns an error.
func readSSE(body io.Reader, onData func(data []byte) (stop bool, err error)) error {
	scanner := bufio.NewScanner(body)
	// Allow large SSE lines (e.g. base64-bearing chunks).
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[len("data:"):])
		if string(data) == "[DONE]" {
			return nil
		}
		stop, err := onData(data)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return scanner.Err()
}

// doStreamRequest issues an HTTP request and returns the response for SSE
// reading. On a non-2xx status it drains the body and returns it as an error.
func doStreamRequest(ctx context.Context, req *http.Request, name string) (*http.Response, error) {
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, errorf(name, resp.StatusCode, raw)
	}
	return resp, nil
}
