package handlers

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/fasthttp/router"
	"github.com/nimasrn/message-gateway/internal/model"
	xhttp "github.com/nimasrn/message-gateway/pkg/http"
)

type MessageService interface {
	Create(ctx context.Context, p model.MessageCreateRequest) (*model.Message, error)
	List(ctx context.Context, f model.MessageFilter) ([]*model.Message, int64, error)
	GetMessagesWithDeliveryReports(ctx context.Context, f model.MessageFilter) ([]*model.MessageWithDeliveryReports, int64, error)
}
type MessageHandler struct {
	svc MessageService
}

func RegisterMessageRoutes(e *router.Group, h *MessageHandler) {
	e.POST("/messages", h.CreateMessage)
	e.POST("/messages/express", h.CreateExpressMessage)
	e.GET("/messages", h.ListMessages)
	e.GET("/messages/delivery-reports", h.ListMessagesWithDeliveryReports)
}

func NewMessageHandler(messageService MessageService) *MessageHandler {
	return &MessageHandler{
		svc: messageService,
	}
}

type createMessageRequest struct {
	CustomerID int64  `json:"customer_id"`
	Mobile     string `json:"mobile"`
	Content    string `json:"content"`
}

type listResponse struct {
	Items []*model.Message `json:"items"`
	Total int64            `json:"total"`
}

type listWithReportResponse struct {
	Items []*model.MessageWithDeliveryReports `json:"items"`
	Total int64                               `json:"total"`
}

/* --------------------------------- Routes ----------------------------------- */

func (h *MessageHandler) ListMessagesWithDeliveryReports(ctx *xhttp.RequestCtx) {
	var (
		f   model.MessageFilter
		err error
	)

	// Parse filters (same as ListMessages)
	if v := query(ctx, "customer_id"); v != "" {
		if id, e := strconv.ParseInt(v, 10, 64); e == nil {
			f.CustomerID = &id
		}
	}
	if v := query(ctx, "mobile"); v != "" {
		f.Mobile = &v
	}
	if v := query(ctx, "status"); v != "" {
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
			if parts[i] != "" {
				f.Statuses = append(f.Statuses, model.MessageStatus(parts[i]))
			}
		}
	}
	if v := query(ctx, "from"); v != "" {
		if t, e := parseTime(v); e == nil {
			f.From = &t
		}
	}
	if v := query(ctx, "to"); v != "" {
		if t, e := parseTime(v); e == nil {
			f.To = &t
		}
	}
	if v := query(ctx, "limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			f.Limit = n
		}
	}
	if v := query(ctx, "offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			f.Offset = n
		}
	}
	if strings.EqualFold(query(ctx, "order"), "desc") {
		f.Desc = true
	}

	items, total, err := h.svc.GetMessagesWithDeliveryReports(ctx, f)
	if err != nil {
		writeError(ctx, 400, err.Error())
		return
	}

	writeJSON(ctx, 200, listWithReportResponse{Items: items, Total: total})
}

func (h *MessageHandler) CreateMessage(ctx *xhttp.RequestCtx) {
	var req createMessageRequest
	if err := readJSON(ctx, &req); err != nil {
		writeError(ctx, 400, "invalid JSON: "+err.Error())
		return
	}
	p := model.MessageCreateRequest{
		CustomerID: req.CustomerID,
		Mobile:     req.Mobile,
		Content:    req.Content,
		Priority:   "normal",
	}
	msg, err := h.svc.Create(ctx, p)
	if err != nil {
		writeError(ctx, 400, err.Error())
		return
	}
	writeJSON(ctx, 201, msg)
}

func (h *MessageHandler) CreateExpressMessage(ctx *xhttp.RequestCtx) {
	var req createMessageRequest
	if err := readJSON(ctx, &req); err != nil {
		writeError(ctx, 400, "invalid JSON: "+err.Error())
		return
	}
	p := model.MessageCreateRequest{
		CustomerID: req.CustomerID,
		Mobile:     req.Mobile,
		Content:    req.Content,
		Priority:   "express",
	}
	msg, err := h.svc.Create(ctx, p)
	if err != nil {
		writeError(ctx, 400, err.Error())
		return
	}
	writeJSON(ctx, 201, msg)
}

func (h *MessageHandler) ListMessages(ctx *xhttp.RequestCtx) {
	var (
		f   model.MessageFilter
		err error
	)

	if v := query(ctx, "customer_id"); v != "" {
		if id, e := strconv.ParseInt(v, 10, 64); e == nil {
			f.CustomerID = &id
		}
	}
	if v := query(ctx, "mobile"); v != "" {
		f.Mobile = &v
	}
	if v := query(ctx, "status"); v != "" {
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
			if parts[i] != "" {
				f.Statuses = append(f.Statuses, model.MessageStatus(parts[i]))
			}
		}
	}
	if v := query(ctx, "from"); v != "" {
		if t, e := parseTime(v); e == nil {
			f.From = &t
		}
	}
	if v := query(ctx, "to"); v != "" {
		if t, e := parseTime(v); e == nil {
			f.To = &t
		}
	}
	if v := query(ctx, "limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			f.Limit = n
		}
	}
	if v := query(ctx, "offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			f.Offset = n
		}
	}
	if strings.EqualFold(query(ctx, "order"), "desc") {
		f.Desc = true
	}

	items, total, err := h.svc.List(ctx, f)
	if err != nil {
		writeError(ctx, 400, err.Error())
		return
	}
	writeJSON(ctx, 200, listResponse{Items: items, Total: total})
}

func readJSON(ctx *xhttp.RequestCtx, dst any) error {
	body := ctx.PostBody()
	return json.Unmarshal(body, dst)
}

func writeJSON(ctx *xhttp.RequestCtx, status int, v any) {
	b, _ := json.Marshal(v)
	ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
	ctx.Response.SetStatusCode(status)
	ctx.Response.SetBodyRaw(b)
}

func writeError(ctx *xhttp.RequestCtx, status int, msg string) {
	writeJSON(ctx, status, map[string]string{"error": msg})
}

func paramInt64(ctx *xhttp.RequestCtx, name string) (int64, error) {
	idStr := ctx.QueryArgs().Peek(name)
	return strconv.ParseInt(string(idStr), 10, 64)
}

func query(ctx *xhttp.RequestCtx, key string) string {
	return string(ctx.QueryArgs().Peek(key))
}

func parseTime(s string) (time.Time, error) {
	// Accept RFC3339 or YYYY-MM-DD
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}
