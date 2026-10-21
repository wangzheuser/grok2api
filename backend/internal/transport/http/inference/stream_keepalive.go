package inference

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	"github.com/gin-gonic/gin"
)

const streamKeepaliveInterval = 15 * time.Second
const streamProtocolKey = "inferenceStreamProtocol"
const streamModelKey = "inferenceStreamModel"

// serveInference keeps a validated streaming request alive while routing,
// upstream headers, or the quality hold are still pending.
func (h *Handler) serveInference(c *gin.Context, input gateway.Input, stream bool, protocol streamProtocol, invoke func(context.Context, gateway.Input) (*gateway.Result, error)) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Set(streamModelKey, input.PublicModel)
	result, err := awaitInference(c, ctx, stream, protocol, streamKeepaliveInterval, func() (*gateway.Result, error) {
		return invoke(ctx, input)
	})
	if err != nil {
		if protocol == streamProtocolAnthropic {
			writeGatewayAnthropicError(c, err)
		} else {
			writeGatewayError(c, err)
		}
		return
	}
	h.writeProtocolResult(c, result, stream, protocol == streamProtocolAnthropic, protocol, input.PublicModel)
}

// awaitInference leaves immediate errors as HTTP errors; only a slow stream
// commits SSE headers. The request goroutine remains the sole response writer.
func awaitInference(c *gin.Context, ctx context.Context, stream bool, protocol streamProtocol, interval time.Duration, invoke func() (*gateway.Result, error)) (*gateway.Result, error) {
	if !stream {
		return invoke()
	}
	c.Set(streamProtocolKey, protocol)
	type outcome struct {
		result *gateway.Result
		err    error
	}
	done := make(chan struct{})
	defer close(done)
	results := make(chan outcome)
	go func() {
		result, err := invoke()
		select {
		case results <- outcome{result, err}:
		case <-done:
			if result != nil {
				result.Finalize(gateway.Usage{}, "", "request_canceled")
				_ = result.Body.Close()
			}
		}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case value := <-results:
			return value.result, value.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("X-Accel-Buffering", "no")
			if err := setResponseWriteDeadline(c.Writer); err != nil {
				return nil, err
			}
			if _, err := c.Writer.Write([]byte(": keep-alive\n\n")); err != nil {
				return nil, err
			}
			if err := http.NewResponseController(c.Writer).Flush(); err != nil {
				return nil, err
			}
		}
	}
}

// keepaliveBody inserts comments only between complete SSE events, never
// inside a fragmented JSON line. It does not reset upstream idle timers.
type keepaliveBody struct {
	source   io.ReadCloser
	results  chan streamRead
	done     chan struct{}
	ticker   *time.Ticker
	pending  []byte
	readErr  error
	lineSize int
	boundary bool
	close    sync.Once
}

type streamRead struct {
	data []byte
	err  error
}

// newKeepaliveBody bounds read-ahead to one chunk and unblocks its reader on Close.
func newKeepaliveBody(source io.ReadCloser, interval time.Duration) *keepaliveBody {
	body := &keepaliveBody{source: source, results: make(chan streamRead), done: make(chan struct{}), ticker: time.NewTicker(interval), boundary: true}
	go func() {
		for {
			buffer := make([]byte, responseCopyBufferBytes)
			n, err := source.Read(buffer)
			select {
			case body.results <- streamRead{buffer[:n], err}:
			case <-body.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return body
}

// Read preserves upstream framing and reports its original terminal error.
func (b *keepaliveBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	for len(b.pending) == 0 && b.readErr == nil {
		select {
		case next := <-b.results:
			b.pending, b.readErr = next.data, next.err
		case <-b.done:
			return 0, io.ErrClosedPipe
		case <-b.ticker.C:
			if b.boundary {
				b.pending = []byte(": keep-alive\n\n")
			}
		}
	}
	n := copy(buffer, b.pending)
	b.pending = b.pending[n:]
	for _, ch := range buffer[:n] {
		if ch == '\n' {
			b.boundary = b.lineSize == 0
			b.lineSize = 0
		} else if ch != '\r' {
			b.lineSize++
			b.boundary = false
		}
	}
	if len(b.pending) == 0 {
		return n, b.readErr
	}
	return n, nil
}

// Close stops the heartbeat and releases a blocked upstream read.
func (b *keepaliveBody) Close() error {
	var err error
	b.close.Do(func() {
		close(b.done)
		b.ticker.Stop()
		err = b.source.Close()
	})
	return err
}

// writeStartedStreamError emits a protocol terminal error after a heartbeat
// has committed HTTP 200; appending a JSON error would corrupt the SSE stream.
func writeStartedStreamError(c *gin.Context, code, message string) bool {
	value, ok := c.Get(streamProtocolKey)
	if !ok || !c.Writer.Written() {
		return false
	}
	protocol, ok := value.(streamProtocol)
	if !ok {
		return false
	}
	model, _ := c.Get(streamModelKey)
	modelName, _ := model.(string)
	trailer := streamErrorTrailer(protocol, code, message, responseMetadata{}, &responsesCompatState{model: modelName})
	if setResponseWriteDeadline(c.Writer) == nil {
		_, _ = c.Writer.Write(trailer)
		c.Writer.Flush()
	}
	c.Abort()
	return true
}
