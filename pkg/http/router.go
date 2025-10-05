package xhttp

import (
	"github.com/fasthttp/router"
)

type Router = router.Router

// NewRouter returns a new Router
func NewRouter() *Router {
	return router.New()
}

// CreateDefaultRouter returns a new router with the default middleware
// PanicHandler
// NotFoundHandler
// GlobalOPTIONS
// MethodNotAllowed
func CreateDefaultRouter() *Router {
	r := NewRouter()
	r.RedirectFixedPath = true
	r.RedirectTrailingSlash = true
	r.SaveMatchedRoutePath = true
	r.NotFound = NotFoundHandler
	r.MethodNotAllowed = NotFoundHandler
	r.HandleOPTIONS = false
	r.HandleMethodNotAllowed = true
	return r
}

// NotFoundHandler is the default 404 handler
func NotFoundHandler(ctx *RequestCtx) {
	ctx.Error(StatusText(StatusNotFound), StatusNotFound)
}
