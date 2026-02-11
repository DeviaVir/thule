package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/example/thule/internal/orchestrator"
)

type Handler struct {
	secret []byte
	orch   *orchestrator.Service
}

func NewHandler(secret string, orch *orchestrator.Service) *Handler {
	return &Handler{secret: []byte(secret), orch: orch}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}

	if len(h.secret) > 0 {
		signature := strings.TrimPrefix(r.Header.Get("X-Thule-Signature"), "sha256=")
		if !verifySignature(h.secret, body, signature) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var event orchestrator.MergeRequestEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := h.orch.HandleMergeRequestEvent(r.Context(), event); err != nil {
		http.Error(w, fmt.Sprintf("event rejected: %v", err), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"queued"}`))
}

func verifySignature(secret, body []byte, provided string) bool {
	if provided == "" {
		return false
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(provided))
}
