package xhttp

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/prefork"
)

// env list:
// xhttp_SERVER_READ_TIMEOUT
// xhttp_SERVER_WRITE_TIMEOUT
// xhttp_SERVER_REQUEST_TIMEOUT

var (
	defaultReadBufferSize  = 1024 * 4
	defaultWriteBufferSize = 1024 * 4
	defaultReadTimeout     = time.Millisecond * 2500
	defaultWriteTimeout    = time.Millisecond * 2500
	defaultRequestTimeout  = time.Millisecond * 5000
)

func init() {
	// set default read/write buffer size
	read := os.Getenv("xhttp_SERVER_READ_TIMEOUT")
	if len(read) > 0 && read != "0" {
		if v, err := strconv.Atoi(read); err == nil {
			fmt.Println("xhttp pkg: setting value from env: xhttp_SERVER_READ_TIMEOUT")
			defaultReadTimeout = time.Millisecond * time.Duration(v)
		}
	}

	write := os.Getenv("xhttp_SERVER_WRITE_TIMEOUT")
	if len(write) > 0 && write != "0" {
		if v, err := strconv.Atoi(write); err == nil {
			fmt.Println("xhttp pkg: setting value from env: xhttp_SERVER_WRITE_TIMEOUT")
			defaultWriteTimeout = time.Millisecond * time.Duration(v)
		}
	}

	req := os.Getenv("xhttp_SERVER_REQUEST_TIMEOUT")
	if len(req) > 0 && req != "0" {
		if v, err := strconv.Atoi(req); err == nil {
			fmt.Println("xhttp pkg: setting value from env: xhttp_SERVER_REQUEST_TIMEOUT")
			defaultRequestTimeout = time.Millisecond * time.Duration(v)
		}
	}

	readBuffer := os.Getenv("xhttp_SERVER_READ_BUFFER_BYTE")
	if len(readBuffer) > 0 && readBuffer != "0" {
		if v, err := strconv.Atoi(req); err == nil && v > 1024 {
			fmt.Println("xhttp pkg: setting value from env: xhttp_SERVER_READ_BUFFER_BYTE")
			defaultReadBufferSize = v
		}
	}

	writeBuffer := os.Getenv("xhttp_SERVER_WRITE_BUFFER_BYTE")
	if len(writeBuffer) > 0 && writeBuffer != "0" {
		if v, err := strconv.Atoi(req); err == nil && v > 1024 {
			fmt.Println("xhttp pkg: setting value from env: xhttp_SERVER_WRITE_BUFFER_BYTE")
			defaultWriteBufferSize = v
		}
	}
}

var DefaultServerOption = ServerOption{
	Handler: func(ctx *RequestCtx) {
		ctx.Error(StatusText(StatusNotFound), StatusNotFound)
	},
	IdleTimeout:           time.Second * 10,
	MaxIdleWorkerDuration: time.Minute * 1,
	TCPKeepalivePeriod:    time.Minute * 120, // linux default
	MaxRequestBodySize:    4 * 1024 * 1024,   // 4MB
	RequestTimeout:        defaultRequestTimeout,
	ReadBufferSize:        defaultReadBufferSize,  // also, max header size
	WriteBufferSize:       defaultWriteBufferSize, // best memory buffer size, 4KB
	ReadTimeout:           defaultReadTimeout,
	WriteTimeout:          defaultWriteTimeout,
	Concurrency:           30_000,
	//  10,000 concurrent connections per IP, the default is 0, which means unlimited
	// the max open files on linux is 65,535
	// the fasthttp client default max conns per ip is 512
	MaxConnsPerIP: 10_000,
	// 0 means unlimited
	MaxRequestsPerConn: 0,
	ErrorHandler: func(ctx *RequestCtx, err error) {
		ctx.Logger().Printf("[xhttp] error: %s", err)
	},
	DisableKeepalive:                   false,
	TCPKeepalive:                       true,
	ReduceMemoryUsage:                  false,
	GetOnly:                            false,
	DisablePreParseMultipartForm:       true,
	LogAllErrors:                       true,
	SecureErrorLogMessage:              false,
	DisableHeaderNamesNormalizing:      false,
	SleepWhenConcurrencyLimitsExceeded: 100,
	NoDefaultServerHeader:              true,
	NoDefaultDate:                      true,
	NoDefaultContentType:               true,
	KeepHijackedConns:                  false,
	CloseOnShutdown:                    true,
	StreamRequestBody:                  false,
	ConnState:                          nil,
	Logger:                             logger.GetLogger(),
	TLSConfig:                          nil,
	CompressionLevel:                   fasthttp.CompressBestSpeed,
	RecoverThreshold:                   100,
}

type RequestHeader = fasthttp.RequestHeader
type ResponseHeader = fasthttp.ResponseHeader
type Prefork = prefork.Prefork
type Server = fasthttp.Server

type ServerOption struct {
	Handler RequestHandler

	// if we keep open idle connections for too long,
	// we can get too many open files error, so we need to set a max idle time for idle connections
	// 10 seconds is a good value
	// if the open goes above 30,000 on machine with 4 core we could process each request much slower
	// doe to the context switch and the concurrent request that the server should handle
	IdleTimeout time.Duration

	// we could keep the workers alive for a long time, but we need to set a max idle time for workers
	// default is 10 minutes
	MaxIdleWorkerDuration time.Duration

	// we could keep the connections alive for a long time, but waste resources, so we set limit
	// default is 10 minutes
	TCPKeepalivePeriod time.Duration

	// just for security reasons and preventing form any attack
	// default is 4MB
	MaxRequestBodySize int

	// we return the request timeout error if the request takes more than 700ms
	RequestTimeout time.Duration

	// ReadBufferSize is the per-connection buffer size for
	// requests' reading.
	// Default is 1_000_000
	ReadBufferSize int

	// WriteBufferSize is the size of the write buffer used by the ResponseWriter.
	// Default is 1_000_000
	WriteBufferSize int

	// ReadTimeout is the maximum duration for reading the entire request,
	// including the body.
	// By default, there is no timeout.
	// Default is 600 seconds
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response.
	// default is 600 seconds
	WriteTimeout time.Duration

	// Concurrency is the maximum number of concurrent connections to serve.
	// default is 1_000_000
	Concurrency int

	MaxConnsPerIP      int
	MaxRequestsPerConn int

	// ErrorHandler
	ErrorHandler                       func(ctx *RequestCtx, err error)
	HeaderReceived                     func(header *RequestHeader) fasthttp.RequestConfig
	ContinueHandler                    func(header *RequestHeader) bool
	Name                               string
	DisableKeepalive                   bool
	TCPKeepalive                       bool
	ReduceMemoryUsage                  bool
	GetOnly                            bool
	DisablePreParseMultipartForm       bool
	LogAllErrors                       bool
	SecureErrorLogMessage              bool
	DisableHeaderNamesNormalizing      bool
	SleepWhenConcurrencyLimitsExceeded time.Duration
	NoDefaultServerHeader              bool
	NoDefaultDate                      bool
	NoDefaultContentType               bool
	KeepHijackedConns                  bool
	CloseOnShutdown                    bool
	StreamRequestBody                  bool
	ConnState                          func(net.Conn, fasthttp.ConnState)
	Logger                             logger.Logger
	TLSConfig                          *tls.Config
	CompressionLevel                   int
	RecoverThreshold                   int
	AccessLog                          bool
}

type Engine struct {
	*Router
	*Server
	*Prefork
	option ServerOption
	middle []MiddlewareFunc
}

func newServer(options ServerOption) *fasthttp.Server {
	return &fasthttp.Server{
		Handler:                            options.Handler,
		ErrorHandler:                       options.ErrorHandler,
		HeaderReceived:                     options.HeaderReceived,
		ContinueHandler:                    options.ContinueHandler,
		Name:                               options.Name,
		Concurrency:                        options.Concurrency,
		ReadBufferSize:                     options.ReadBufferSize,
		WriteBufferSize:                    options.WriteBufferSize,
		ReadTimeout:                        options.ReadTimeout,
		WriteTimeout:                       options.WriteTimeout,
		IdleTimeout:                        options.IdleTimeout,
		MaxConnsPerIP:                      options.MaxConnsPerIP, // unlimited by default
		MaxRequestsPerConn:                 options.MaxRequestsPerConn,
		MaxIdleWorkerDuration:              options.MaxIdleWorkerDuration,
		TCPKeepalivePeriod:                 options.TCPKeepalivePeriod,
		MaxRequestBodySize:                 options.MaxRequestBodySize,
		DisableKeepalive:                   options.DisableKeepalive,
		TCPKeepalive:                       options.TCPKeepalive,
		ReduceMemoryUsage:                  options.ReduceMemoryUsage,
		GetOnly:                            options.GetOnly,
		DisablePreParseMultipartForm:       options.DisablePreParseMultipartForm,
		LogAllErrors:                       options.LogAllErrors,
		SecureErrorLogMessage:              options.SecureErrorLogMessage,
		DisableHeaderNamesNormalizing:      options.DisableHeaderNamesNormalizing,
		SleepWhenConcurrencyLimitsExceeded: options.SleepWhenConcurrencyLimitsExceeded,
		NoDefaultServerHeader:              options.NoDefaultServerHeader,
		NoDefaultDate:                      options.NoDefaultDate,
		NoDefaultContentType:               options.NoDefaultContentType,
		KeepHijackedConns:                  options.KeepHijackedConns,
		CloseOnShutdown:                    options.CloseOnShutdown,
		StreamRequestBody:                  options.StreamRequestBody,
		ConnState:                          options.ConnState,
		Logger:                             options.Logger,
		TLSConfig:                          options.TLSConfig,
	}
}

func NewServer(options ServerOption) *Engine {
	return &Engine{
		Server: newServer(options),
		Router: NewRouter(),
		option: options,
	}
}

func CreateServer() *Engine {
	s := NewServer(DefaultServerOption)
	s.Router = CreateDefaultRouter()
	s.Server.Logger = logger.GetLogger()
	return s
}

func (e *Engine) ListenAndServe(addr string) error {
	err := e.DoRouting()
	if err != nil {
		return err
	}
	e.Server.Logger.Printf("[xhttp] server is listening on %s", addr)
	if err := e.Server.ListenAndServe(addr); err != nil {
		return err
	}
	return nil
}

func (e *Engine) PreforkListenAndServe(addr string) error {
	err := e.DoRouting()
	if err != nil {
		return err
	}
	e.Prefork = prefork.New(e.Server)
	e.Prefork.Reuseport = true
	e.Prefork.RecoverThreshold = e.option.RecoverThreshold
	e.Prefork.Logger = e.Server.Logger
	e.Prefork.Logger.Printf("[xhttp] server is listening on %s", addr)
	if err := e.Prefork.ListenAndServe(addr); err != nil {
		return err
	}
	return nil
}

func (e *Engine) DoRouting() error {
	// log all registered routes grouped by method
	for method, route := range e.Router.List() {
		for _, r := range route {
			e.Server.Logger.Printf("[xhttp] method: %s, path: %s", method, r)
		}
	}
	// set handler
	e.Server.Handler = e.Router.Handler
	// set middleware
	// reverse the middlewares
	// so the first middleware will be the last to be executed
	// and the last middleware will be the first to be executed
	slices.Reverse(e.middle)
	for i, m := range e.middle {
		e.Server.Handler = m(e.Server.Handler)
		e.Server.Logger.Printf("[xhttp] middleware %d registered - %s", i+1, runtime.FuncForPC(reflect.ValueOf(m).Pointer()).Name())
	}
	return nil
}

func (e *Engine) CloseOnSignal() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig
		if e.Server != nil {
			e.Prefork.RecoverThreshold = 0
		}
		e.Shutdown()
	}()
}

// Use adds middleware to the chain which is run for every request.
//
//	func CORS(next xhttp.RequestHandler) xhttp.RequestHandler {
//		return func(ctx *xhttp.RequestCtx) {
//
//		ctx.Response.Header.Set("Access-Control-Allow-Credentials", corsAllowCredentials)
//		ctx.Response.Header.Set("Access-Control-Allow-Headers", corsAllowHeaders)
//		ctx.Response.Header.Set("Access-Control-Allow-Methods", corsAllowMethods)
//		ctx.Response.Header.Set("Access-Control-Allow-Origin", corsAllowOrigin)
//
//		next(ctx)
//		}
//	}
func (e *Engine) Use(middleware MiddlewareFunc) {
	// add middleware to the end of the chain
	e.middle = append(e.middle, middleware)
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
// example:
// sig := make(chan os.Signal, 1)
// signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
//
//	go func() {
//		<-sig
//		s.server.Shutdown()
//	}()
func (e *Engine) Shutdown() {
	// shutdown
	e.Server.Logger.Printf("[xhttp] server is shutting down, process id: %d isChild: %v", os.Getpid(), prefork.IsChild())
	if e.Prefork != nil {
		e.Prefork.RecoverThreshold = 0
	}
	e.Server.Logger.Printf("[xhttp] closing all connections..")
	err := e.Server.Shutdown()
	if err != nil {
		e.Server.Logger.Printf("[xhttp] error while shutting down: %v", err)
	}
}
