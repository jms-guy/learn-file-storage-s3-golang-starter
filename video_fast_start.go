package main

import "os/exec"

func processVideoForFastStart(filepath string) (string, error) {
	output := filepath+".processing"
	cmd := exec.Command("ffmpeg", "-i", filepath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", output)

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return output, nil
}