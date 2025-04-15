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

	file, header, err := r.FormFile("video")	//Returns multipart form file from video section of request body
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")	//Get content type header from request

	t, _, err := mime.ParseMediaType(mediaType)	//Parse the mime type of the content header
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get media type", err)
		return
	}

	if t != "video/mp4" {	//Media type must be mp4 file
		respondWithError(w, http.StatusBadRequest, "Unsupported media type", fmt.Errorf("unsupported media type"))
		return
	}

	tFile, err := os.CreateTemp("", "tubely-upload.mp4")	//Create temp file to store multipart form data 
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating temp file", err)
		return
	}
	defer os.Remove(tFile.Name())	//Defer remove and close of temp file
	defer tFile.Close()

	_, err = io.Copy(tFile, file)	//Copy multipart file into temp file on disk
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error copying data to temp file", err)
		return
	}

	_, err = tFile.Seek(0, io.SeekStart)	//Reset read pointer of temp file to beginning of file
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reseting temp file read pointer", err)
		return
	}

	fastVideo, err := processVideoForFastStart(tFile.Name())	//Sets the moov Atom of file to beginning for fast video starting
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error setting moov Atom of video: "+tFile.Name(), err)
		return
	}

	processedFile, err := os.Open(fastVideo)	//Open the processed file of the fast start video
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error opening processed video: "+fastVideo, err)
		return
	}
	defer processedFile.Close()

	ratio, err := getVideoAspectRatio(tFile.Name())	//Get aspect ratio of video file from dimensions
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error getting aspect ratio", err)
		return
	}

	var videoType string	//Set video type
	if ratio == "16:9" {
		videoType = "landscape"
	} else if ratio == "9:16" {
		videoType = "portrait"
	} else {
		videoType = "other"
	}

	var extension string	//Get file extension 
	parts := strings.Split(mediaType, "/")
	if len(parts) == 2 {
		extension = "." + parts[1]
	}

	b := make([]byte, 32)	//Make random 32-byte slice
	_, err = rand.Read(b)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating byte slice", err)
		return
	}
	id := base64.RawURLEncoding.EncodeToString(b)	//Encode 32-byte slice to string for random path name

	fileName := videoType+"/"+id+extension	//Put together file name

	objParams := s3.PutObjectInput{	//Create S3 putobject params to pass into the S3 bucket
		Bucket: &cfg.s3Bucket,
		Key: &fileName,
		Body: processedFile,
		ContentType: &mediaType,
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &objParams)	//Pass video object into S3 bucket
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error storing object", err)
		return
	}

	videoURL := cfg.s3CfDistribution+"/"+fileName	
	metaData.VideoURL = &videoURL	//Set video url metadata 

	err = cfg.db.UpdateVideo(metaData)	//Update the video metadata on the database
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update database metadata", err)
		return
	}
	
	respondWithJSON(w, http.StatusOK, metaData)	//Respond OK
}
