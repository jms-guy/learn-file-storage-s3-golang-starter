package main

import "os/exec"

func processVideoForFastStart(filepath string) (string, error) {
	output := filepath+".processing"	//Create new filepath for processed video file
	cmd := exec.Command("ffmpeg", "-i", filepath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", output)	//Create ffmpeg shell command

	err := cmd.Run()	//Run command to set moov Atom of file to beginning
	if err != nil {
		return "", err
	}

	return output, nil
}