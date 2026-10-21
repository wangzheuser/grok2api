package inference

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	"github.com/gin-gonic/gin"
)

// TestKeepaliveBodyPreservesFragmentedEvents covers boundaries, comments and EOF.
func TestKeepaliveBodyPreservesFragmentedEvents(t *testing.T) {
	reader, writer := io.Pipe()
	body := newKeepaliveBody(reader, 5*time.Millisecond)
	defer body.Close()
	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(writer, "data: {\"value\":")
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(writer, "1}\n\n")
		time.Sleep(20 * time.Millisecond)
		_ = writer.Close()
	}()
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	output := string(raw)
	if !strings.Contains(output, ": keep-alive\n\n") {
		t.Fatalf("no heartbeat: %q", output)
	}
	if stripped := strings.ReplaceAll(output, ": keep-alive\n\n", ""); stripped != "data: {\"value\":1}\n\n" {
		t.Fatalf("upstream bytes changed: %q", output)
	}
	if !strings.Contains(output, "data: {\"value\":1}\n\n") {
		t.Fatalf("heartbeat split JSON: %q", output)
	}
}

// TestKeepaliveBodyCloseStopsBlockedRead checks cancellation without a leaked read.
func TestKeepaliveBodyCloseStopsBlockedRead(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	body := newKeepaliveBody(reader, time.Hour)
	done := make(chan error, 1)
	go func() { _, err := body.Read(make([]byte, 10)); done <- err }()
	_ = body.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("read did not stop")
	}
}

// TestAwaitInferenceKeepsSlowErrorsInSSE preserves each protocol's terminal error.
func TestAwaitInferenceKeepsSlowErrorsInSSE(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol streamProtocol
		want     string
	}{
		{"responses", streamProtocolResponses, "event: response.failed"},
		{"chat", streamProtocolChat, "data: [DONE]"},
		{"messages", streamProtocolAnthropic, "event: error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(streamModelKey, "grok-test")
			_, err := awaitInference(c, context.Background(), true, tc.protocol, time.Millisecond, func() (*gateway.Result, error) {
				time.Sleep(20 * time.Millisecond)
				return nil, errors.New("upstream failure")
			})
			if err == nil {
				t.Fatal("missing error")
			}
			writeOpenAIError(c, http.StatusBadGateway, "upstream_failed", "upstream failure")
			if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), ": keep-alive") || !strings.Contains(recorder.Body.String(), tc.want) {
				t.Fatalf("response=%d %q", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestAwaitInferencePreservesImmediateHTTPError covers nonstream and fast failures.
func TestAwaitInferencePreservesImmediateHTTPError(t *testing.T) {
	for _, stream := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		_, _ = awaitInference(c, context.Background(), stream, streamProtocolChat, time.Hour, func() (*gateway.Result, error) {
			return nil, errors.New("invalid model")
		})
		writeOpenAIError(c, http.StatusBadRequest, "invalid_model", "invalid model")
		if recorder.Code != 400 || strings.Contains(recorder.Body.String(), "keep-alive") {
			t.Fatalf("response=%d %q", recorder.Code, recorder.Body.String())
		}
	}
}

// TestAwaitInferenceFinalizesLateResult ensures cancellation releases reservations.
func TestAwaitInferenceFinalizesLateResult(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan string, 1)
	release := make(chan struct{})
	invoked := make(chan struct{})
	go func() { <-invoked; cancel() }()
	_, err := awaitInference(c, ctx, true, streamProtocolChat, time.Hour, func() (*gateway.Result, error) {
		close(invoked)
		<-release
		return &gateway.Result{
			Body:     io.NopCloser(strings.NewReader("")),
			Finalize: func(_ gateway.Usage, _, code string) { finished <- code },
		}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	close(release)
	select {
	case code := <-finished:
		if code != "request_canceled" {
			t.Fatalf("code=%s", code)
		}
	case <-time.After(time.Second):
		t.Fatal("late result leaked")
	}
}
