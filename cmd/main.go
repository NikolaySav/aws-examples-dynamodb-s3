package main

import (
	"aws-examples/internal/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	"log"
)

func main() {

	sess := session.Must(session.NewSession(&aws.Config{
		Region:           aws.String("us-east-1"),              // Matches your LocalStack AWS_REGION
		Endpoint:         aws.String("http://localstack:4566"), // LocalStack endpoint
		S3ForcePathStyle: aws.Bool(true),                       // Required for LocalStack
	}))
	fileStorage := s3.New(sess)

	sess2 := session.Must(session.NewSession(&aws.Config{
		Region:   aws.String("us-east-1"),              // Matches your LocalStack AWS_REGION
		Endpoint: aws.String("http://localstack:4566"), // LocalStack endpoint
	}))
	db := dynamodb.New(sess2)

	// CreateFile the service
	service := app.NewService(
		fileStorage,
		"file-storage-bucket",
		db,
		"file-storage-table",
	)

	// Run the service
	if err := service.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
