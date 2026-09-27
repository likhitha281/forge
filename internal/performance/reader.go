package performance

import (
	"encoding/json"
	"os"
	"time"
)

type progressFile struct {
	Completed  uint64  `json:"completed"`
	Total      uint64  `json:"total"`
	Allocation int     `json:"allocation"`
	Rate       float64 `json:"rate"`
	Timestamp  string  `json:"timestamp"`
}

func ReadSnapshot(
	path string,
) (Snapshot, error) {
	data, err :=
		os.ReadFile(path)

	if err != nil {
		return Snapshot{}, err
	}

	var progress progressFile

	if err := json.Unmarshal(
		data,
		&progress,
	); err != nil {
		return Snapshot{}, err
	}

	var timestamp time.Time

	if progress.Timestamp != "" {
		timestamp, err =
			time.Parse(
				time.RFC3339Nano,
				progress.Timestamp,
			)

		if err != nil {
			return Snapshot{}, err
		}
	}

	snapshot := Snapshot{
		Completed:  progress.Completed,
		Total:      progress.Total,
		Allocation: progress.Allocation,
		Rate:       progress.Rate,
		Timestamp:  timestamp,
	}

	if snapshot.Total == 0 ||
		snapshot.Completed > snapshot.Total ||
		snapshot.Allocation <= 0 ||
		snapshot.Rate < 0 {

		return Snapshot{},
			ErrInvalidSnapshot
	}

	return snapshot, nil
}
