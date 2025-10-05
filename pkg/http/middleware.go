package xhttp

import (
	"strings"
	"time"

	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/valyala/fasthttp"
)

const slowThreshold = 500 * time.Millisecond

var skipPaths = []string{"/health", "/metrics"}

type MiddlewareFunc func(next RequestHandler) RequestHandler
type RequestCtx = fasthttp.RequestCtx
type RequestHandler = fasthttp.RequestHandler

func TimeoutMiddleware(timeout time.Duration) MiddlewareFunc {
	return func(next RequestHandler) RequestHandler {
		return fasthttp.TimeoutWithCodeHandler(next, timeout, StatusText(StatusRequestTimeout), StatusRequestTimeout)
	}
}

func CompressMiddleware(level int) MiddlewareFunc {
	return func(next RequestHandler) RequestHandler {
		return fasthttp.CompressHandlerBrotliLevel(next, level, level)
	}
}

func RecoverMiddleware(next RequestHandler) RequestHandler {
	return func(ctx *RequestCtx) {
		defer func() {
			if err := recover(); err != nil {
				ctx.Error(StatusText(StatusInternalServerError), StatusInternalServerError)
				ctx.Logger().Printf("panic: %v", err)
				logger.Error("[xhttp] panic recovered", "error", err)
			}
		}()
		next(ctx)
	}
}

func RequestLoggerMiddleware(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		if shouldSkip(path) {
			next(ctx)
			return
		}

		start := time.Now()
		next(ctx)

		latency := time.Since(start)
		status := ctx.Response.StatusCode()
		method := string(ctx.Method())
		ip := ctx.RemoteIP().String()
		ua := string(ctx.Request.Header.UserAgent())
		rid := requestID(ctx)

		lg := logger.GetLogger()

		// choose level
		switch {
		case status >= 500:
			lg.Error("http_request",
				"status", status,
				"method", method,
				"path", path,
				"latency", latency.String(),
				"bytes_in", len(ctx.PostBody()),
				"bytes_out", len(ctx.Response.Body()),
				"ip", ip,
				"ua", ua,
				"request_id", rid,
			)
		case status >= 400 || latency > slowThreshold:
			lg.Warn("http_request",
				"status", status,
				"method", method,
				"path", path,
				"latency", latency.String(),
				"bytes_in", len(ctx.PostBody()),
				"bytes_out", len(ctx.Response.Body()),
				"ip", ip,
				"ua", ua,
				"request_id", rid,
			)
		default:
			lg.Info("http_request",
				"status", status,
				"method", method,
				"path", path,
				"latency", latency.String(),
				"bytes_in", len(ctx.PostBody()),
				"bytes_out", len(ctx.Response.Body()),
				"ip", ip,
				"ua", ua,
				"request_id", rid,
			)
		}
	}
}

func shouldSkip(p string) bool {
	for _, sp := range skipPaths {
		if strings.HasPrefix(p, sp) {
			return true
		}
	}
	return false
}

func requestID(ctx *fasthttp.RequestCtx) string {
	if v := ctx.Request.Header.Peek("X-Request-Id"); len(v) > 0 {
		return string(v)
	}
	if v := ctx.Request.Header.Peek("X-Request-ID"); len(v) > 0 { // common variant
		return string(v)
	}
	return ""
}
