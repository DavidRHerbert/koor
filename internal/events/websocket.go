package events

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"nhooyr.io/websocket"
)

// ServeSubscribe handles WebSocket subscription connections.
// Query params:
//   - pattern: glob pattern for topic filtering (default: "*")
func ServeSubscribe(bus *Bus, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pattern := r.URL.Query().Get("pattern")
		if pattern == "" {
			pattern = "*"
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // Allow any origin for local dev.
		})
		if err != nil {
			logger.Error("websocket accept failed", "error", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "closing")

		logger.Info("websocket subscriber connected", "pattern", pattern, "remote", r.RemoteAddr)

		sub := bus.Subscribe(pattern)
		defer bus.Unsubscribe(sub)

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub.Ch:
				if !ok {
					return
				}
				data, err := json.Marshal(ev)
				if err != nil {
					logger.Error("marshal event failed", "error", err)
					continue
				}
				if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
					logger.Debug("websocket write failed", "error", err)
					return
				}
			}
		}
	}
}
