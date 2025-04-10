package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")

	metaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error fetching video metadata", err)
		return
	}

	if userID != metaData.CreateVideoParams.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", fmt.Errorf("Unauthorized"))
		return
	}

	t, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get media type", err)
		return
	}
	if (t != "image/jpeg") && (t != "image/png") {
		respondWithError(w, http.StatusBadRequest, "Unsupported media type", fmt.Errorf("unsupported media type"))
		return
	}

	var extension string
	parts := strings.Split(mediaType, "/")
	if len(parts) == 2 {
		extension = "." + parts[1]
		// Now you have ".png" or ".jpeg"
	}
	fileName := videoID.String()+extension
	filePath := filepath.Join(cfg.assetsRoot, fileName)
	destFile, fileErr := os.Create(filePath)
	if fileErr != nil {
		respondWithError(w, http.StatusInternalServerError, "Error writing media file", fileErr)
		return
	}
	defer destFile.Close()

	if _, cErr := io.Copy(destFile, file); cErr != nil {
		respondWithError(w, http.StatusInternalServerError, "Error copying file data", cErr)
	}

	url := "http://localhost:8091/assets/"+fileName
	metaData.ThumbnailURL = &url

	err = cfg.db.UpdateVideo(metaData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update database metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metaData)
}
