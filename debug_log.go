package re3

import (
	"encoding/json"
	"os"
	"time"
)

// #region agent log
func agentDebugLog(hypothesisId, location, message string, data map[string]any) {
	entry := map[string]any{
		"sessionId":    "5fe40b",
		"runId":        "runtime-1",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile("/Users/binaek/personal_projects/re3/.cursor/debug-5fe40b.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// #endregion
