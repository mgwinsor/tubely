package main

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

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

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find video in database", err)
		return
	}
	if userID != videoMetadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "User does not own the video", err)
		return
	}

	video, videoHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read video file", err)
		return
	}
	defer video.Close()

	contentType := videoHeader.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}
	defer os.Remove("")
	defer tempFile.Close()

	_, err = io.Copy(tempFile, video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}

	videoKey := videoMetadata.ID.String() + ".mp4"
	s3PutObjectInput := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         aws.String(videoKey),
		Body:        tempFile,
		ContentType: aws.String("video/mp4"),
	}
	_, err = cfg.s3Client.PutObject(context.Background(), &s3PutObjectInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}

	videoURL := fmt.Sprintf("https://tubely-5566.s3.%s.amazonaws.com/%s", cfg.s3Region, videoKey)
	videoMetadata.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}
}
