package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	router            *mux.Router
	fileStorage       *s3.S3
	fileStorageBucket string
	db                *dynamodb.DynamoDB
	dbFileTableName   string
}

func NewService(
	fileStorage *s3.S3,
	fileStorageBucket string,
	db *dynamodb.DynamoDB,
	dbFileTableName string,
) *Service {
	service := &Service{
		router:            mux.NewRouter(),
		fileStorage:       fileStorage,
		fileStorageBucket: fileStorageBucket,
		db:                db,
		dbFileTableName:   dbFileTableName,
	}
	service.routes()
	return service
}

func (s *Service) routes() {
	s.router.HandleFunc("/file/{id}", s.GetFile).Methods(http.MethodGet)
	s.router.HandleFunc("/file/{id}", s.DeleteFile).Methods(http.MethodDelete)
	s.router.HandleFunc("/file", s.CreateFile).Methods(http.MethodPost)
}

func (s *Service) Run(port string) error {
	fmt.Printf("Starting server on %s...\n", port)
	return http.ListenAndServe(port, s.router)
}

type FileMetadata struct {
	ID        string `json:"id" dynamodbav:"ID"`
	Hash      string `json:"hash" dynamodbav:"Hash"`
	Extension string `json:"extension" dynamodbav:"Extension"`
	CreatedAt string `json:"created_at" dynamodbav:"CreatedAt"`
	UpdatedAt string `json:"updated_at" dynamodbav:"UpdatedAt"`
}

func validateFile(file io.Reader, fileHeader string) (string, error) {
	ext := filepath.Ext(fileHeader)
	if ext != ".jpeg" && ext != ".jpg" {
		return "", fmt.Errorf("only JPEG files are allowed")
	}

	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if mimeType := http.DetectContentType(buffer); mimeType != "image/jpeg" {
		return "", fmt.Errorf("file is not a valid JPEG image")
	}
	return ext, nil
}

func (s *Service) generatePresignedURL(objectKey string) (string, error) {
	req, _ := s.fileStorage.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.fileStorageBucket),
		Key:    aws.String(objectKey),
	})

	presignedURL, err := req.Presign(15 * time.Minute)
	if err != nil {
		return "", err
	}

	presignedURL = replaceLocalstackHostWithLocalhost(presignedURL)

	return presignedURL, nil
}

func (s *Service) uploadToS3(objectKey string, fileBuffer []byte) error {
	_, err := s.fileStorage.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s.fileStorageBucket),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader(fileBuffer),
	})
	return err
}

func (s *Service) saveMetadataToDB(metadata FileMetadata) error {
	if metadata.ID == "" || metadata.Hash == "" {
		return fmt.Errorf("metadata must have non-empty ID and Hash")
	}

	item, err := dynamodbattribute.MarshalMap(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	_, err = s.db.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(s.dbFileTableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to save metadata to DynamoDB: %w", err)
	}

	return nil
}

func (s *Service) retrieveMetadataFromDB(id string) (*FileMetadata, error) {
	result, err := s.db.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(s.dbFileTableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ID": {S: aws.String(id)},
		},
	})
	if err != nil || result.Item == nil {
		return nil, err
	}
	var metadata FileMetadata
	err = dynamodbattribute.UnmarshalMap(result.Item, &metadata)
	return &metadata, err
}

type FileResponse struct {
	Metadata     *FileMetadata `json:"metadata"`
	PresignedURL string        `json:"presigned_url"`
}

func (s *Service) CreateFile(w http.ResponseWriter, r *http.Request) {
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext, err := validateFile(file, fileHeader.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	file.Seek(0, io.SeekStart)

	fileBuffer := new(bytes.Buffer)
	_, err = io.Copy(fileBuffer, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hash := calculateHash(fileBuffer.Bytes())
	existingFile, err := s.getFileIDByHash(hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if existingFile != nil {
		presignedURL, err := s.generatePresignedURL(existingFile.ID + existingFile.Extension)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(FileResponse{
			Metadata:     existingFile,
			PresignedURL: presignedURL,
		})
		return
	}

	id := uuid.New().String()
	objectKey := id + ext
	now := time.Now().Format(time.RFC3339)
	metadata := FileMetadata{
		ID:        id,
		Hash:      hash,
		Extension: ext,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.uploadToS3(objectKey, fileBuffer.Bytes()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.saveMetadataToDB(metadata); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	presignedURL, err := s.generatePresignedURL(objectKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(FileResponse{
		Metadata:     &metadata,
		PresignedURL: presignedURL,
	})
}

func (s *Service) GetFile(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	metadata, err := s.retrieveMetadataFromDB(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if metadata == nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	objectKey := metadata.ID + metadata.Extension
	presignedURL, err := s.generatePresignedURL(objectKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(FileResponse{
		Metadata:     metadata,
		PresignedURL: presignedURL,
	})
}

func (s *Service) DeleteFile(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	metadata, err := s.retrieveMetadataFromDB(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	objectKey := metadata.ID + metadata.Extension
	_, err = s.fileStorage.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.fileStorageBucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = s.db.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(s.dbFileTableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ID": {S: aws.String(id)},
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func calculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (s *Service) getFileIDByHash(hash string) (*FileMetadata, error) {
	if hash == "" {
		return nil, fmt.Errorf("hash cannot be empty")
	}

	result, err := s.db.Query(&dynamodb.QueryInput{
		TableName:              aws.String(s.dbFileTableName),
		IndexName:              aws.String("HashIndex"),
		KeyConditionExpression: aws.String("#hash = :hash"),
		ExpressionAttributeNames: map[string]*string{
			"#hash": aws.String("Hash"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":hash": {S: aws.String(hash)},
		},
		Limit: aws.Int64(1),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query DynamoDB: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil
	}

	var metadata FileMetadata
	err = dynamodbattribute.UnmarshalMap(result.Items[0], &metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal query result: %w", err)
	}

	return &metadata, nil
}

func replaceLocalstackHostWithLocalhost(url string) string {
	return strings.Replace(url, "http://localstack:4566", "http://localhost:4566", 1)
}
