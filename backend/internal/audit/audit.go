package audit

import (
	"context"

	"github.com/stillmind/backend/internal/memory"
)

type Logger struct {
	Store *memory.Store
}

func (l *Logger) Write(ctx context.Context, userID, typ string, payload any) {
	if l == nil {
		return
	}
	b, _ := jsonMarshal(payload)
	_ = l.Store.Audit(ctx, userID, typ, b)
}

func jsonMarshal(v any) ([]byte, error) { return jsonM(v) }
