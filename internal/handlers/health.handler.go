package handlers

import (
	"github.com/fasthttp/router"
	xhttp "github.com/nimasrn/message-gateway/pkg/http"
)

type HealthService interface {
	Get() error
}
type HealthHandler struct {
	messageService HealthService
}

func RegisterHealthRoutes(e *router.Group, h *HealthHandler) {
	e.GET("/health", h.GetHealth)
}

func NewHealthHandler(messageService HealthService) *HealthHandler {
	return &HealthHandler{
		messageService: messageService,
	}
}

func (h *HealthHandler) GetHealth(ctx *xhttp.RequestCtx) {
	ctx.Response.SetBodyString("success")
	return
}
