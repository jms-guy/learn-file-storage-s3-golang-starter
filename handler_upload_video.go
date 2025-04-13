package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")	//Get video ID
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)	//Get bearer token from header
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)	//Authorize based on token
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	const maxMemory = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

	metaData, err := cfg.db.GetVideo(videoID)	//Get video metadata from the database
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error fetching video metadata", err)
		return
	}

	if userID != metaData.CreateVideoParams.UserID {	//User authorization
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", fmt.Errorf("Unauthorized"))
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")

	t, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get media type", err)
		return
	}

	if t != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Unsupported media type", fmt.Errorf("unsupported media type"))
		return
	}

	tFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating temp file", err)
		return
	}
	defer os.Remove(tFile.Name())
	defer tFile.Close()

	_, err = io.Copy(tFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error copying data to temp file", err)
		return
	}

	_, err = tFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reseting temp file read pointer", err)
		return
	}

	ratio, err := getVideoAspectRatio(tFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error getting aspect ratio", err)
		return
	}

	var videoType string
	if ratio == "16:9" {
		videoType = "landscape"
	} else if ratio == "9:16" {
		videoType = "portrait"
	} else {
		videoType = "other"
	}

	var extension string
	parts := strings.Split(mediaType, "/")
	if len(parts) == 2 {
		extension = "." + parts[1]
	}

	b := make([]byte, 32)
	_, err = rand.Read(b)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating byte slice", err)
		return
	}
	id := base64.RawURLEncoding.EncodeToString(b)

	fileName := videoType+"/"+id+extension

	objParams := s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &fileName,
		Body: tFile,
		ContentType: &mediaType,
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &objParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error storing object", err)
		return
	}

	url := "https://"+cfg.s3Bucket+".s3."+cfg.s3Region+".amazonaws.com/"+fileName
	metaData.VideoURL = &url

	err = cfg.db.UpdateVideo(metaData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update database metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metaData)
}
