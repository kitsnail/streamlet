package handlers

import (
	"fmt"
	"os"
	"time"

	"github.com/abema/go-mp4"
)

// GetMP4Duration returns the duration of an MP4 file
func GetMP4Duration(filePath string) (time.Duration, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Parse MP4 file looking for mvhd box
	var duration time.Duration

	_, err = mp4.ReadBoxStructure(file, func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type.String() {
		case "moov":
			return h.Expand()
		case "mvhd":
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			if mvhd, ok := box.(*mp4.Mvhd); ok {
				durationSeconds := float64(mvhd.GetDuration()) / float64(mvhd.Timescale)
				duration = time.Duration(durationSeconds * float64(time.Second))
				return duration, nil
			}
		}
		return nil, nil
	})

	if err != nil && duration == 0 {
		return 0, fmt.Errorf("failed to get duration: %w", err)
	}

	return duration, nil
}

// FormatDuration formats duration to human readable string (HH:MM:SS or MM:SS)
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
