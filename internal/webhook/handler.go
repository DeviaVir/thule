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

	event, err := decodeEvent(body)
	if err != nil {
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

func decodeEvent(body []byte) (orchestrator.MergeRequestEvent, error) {
	var direct orchestrator.MergeRequestEvent
	if err := json.Unmarshal(body, &direct); err == nil && direct.DeliveryID != "" && direct.MergeReqID > 0 {
		return direct, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return orchestrator.MergeRequestEvent{}, err
	}

	kind, _ := payload["object_kind"].(string)
	deliveryID := str(payload["event_id"])
	if deliveryID == "" {
		deliveryID = str(payload["delivery_id"])
	}
	repo := ""
	if p, ok := payload["project"].(map[string]any); ok {
		repo = str(p["path_with_namespace"])
	}
	changed := []string{}
	if arr, ok := payload["changed_files"].([]any); ok {
		for _, v := range arr {
			changed = append(changed, str(v))
		}
	}

	switch kind {
	case "merge_request":
		attrs, _ := payload["object_attributes"].(map[string]any)
		mrID := int64(num(attrs["iid"]))
		head := ""
		if c, ok := attrs["last_commit"].(map[string]any); ok {
			head = str(c["id"])
		}
		if head == "" {
			head = str(payload["head_sha"])
		}
		eventType := "merge_request.updated"
		action := strings.ToLower(str(attrs["action"]))
		state := strings.ToLower(str(attrs["state"]))
		if action == "close" || state == "closed" {
			eventType = "merge_request.closed"
		} else if action == "merge" || state == "merged" {
			eventType = "merge_request.merged"
		}
		return orchestrator.MergeRequestEvent{DeliveryID: deliveryID, EventType: eventType, Repository: repo, MergeReqID: mrID, HeadSHA: head, ChangedFiles: changed}, nil
	case "note":
		attrs, _ := payload["object_attributes"].(map[string]any)
		note := strings.TrimSpace(str(attrs["note"]))
		if !strings.HasPrefix(note, "/thule plan") {
			return orchestrator.MergeRequestEvent{}, fmt.Errorf("unsupported note command")
		}
		mr, _ := payload["merge_request"].(map[string]any)
		mrID := int64(num(mr["iid"]))
		head := str(mr["last_commit"]) // fallback textual
		if head == "" {
			head = str(payload["head_sha"])
		}
		return orchestrator.MergeRequestEvent{DeliveryID: deliveryID, EventType: "comment.plan", Repository: repo, MergeReqID: mrID, HeadSHA: head, ChangedFiles: changed}, nil
	default:
		return orchestrator.MergeRequestEvent{}, fmt.Errorf("unsupported event kind")
	}
}

func num(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
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
