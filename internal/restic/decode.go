package restic

import (
	"bytes"
	"encoding/json"
)

func DecodeBackupSummary(stream []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(stream))
	var summary map[string]any
	for decoder.More() {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			return nil, err
		}
		if event["message_type"] == "summary" {
			summary = event
		}
	}
	return summary, nil
}
