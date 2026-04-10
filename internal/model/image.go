package model

import (
	"time"
)

type ImageSummary struct {
	ID          string
	Tags        []string
	Size        string
	SizeRaw     int64
	Created     time.Time
	CreatedSince string
}
