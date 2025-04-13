package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

type ProbeStreams struct {
	Streams []struct {
		Width              int    `json:"width,omitempty"`
		Height             int    `json:"height,omitempty"`
	} `json:"streams"`
}

func getVideoAspectRatio(filepath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filepath)

	var b bytes.Buffer
	cmd.Stdout = &b

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	var streams ProbeStreams
	err = json.Unmarshal(b.Bytes(), &streams)
	if err != nil {
		return "", err
	}

	if len(streams.Streams) == 0 {
		return "", fmt.Errorf("no stream structs present in data")
	}

	width := streams.Streams[0].Width
	height := streams.Streams[0].Height

	if width == 0 || height == 0 {
		return "", fmt.Errorf("0 value in dimensions for %s", filepath)
	}

	ratio := float64(width)/float64(height)
	if ratio >= 0.55 && ratio <= 0.57 {
		return "9:16", nil
	} else if ratio >= 1.75 && ratio <= 1.80 {
		return "16:9", nil
	} else {
		return "other", nil
	}
}