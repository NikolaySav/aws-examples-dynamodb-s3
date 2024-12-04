# File Storage Service

This service provides a simple API for uploading, retrieving, and deleting files. Files are stored in an S3-compatible
storage, and their metadata is saved in DynamoDB. The service uses **Gorilla Mux** for routing, AWS SDK for S3 and
DynamoDB operations, and LocalStack for local testing and emulation of AWS services.

---

## Features

- Upload files (JPEG images only) to S3 and save metadata to DynamoDB.
- Retrieve file metadata and a presigned URL for direct file access.
- Delete files from S3 and their metadata from DynamoDB.
- Generate presigned URLs to securely access files.

---

## Prerequisites

- [Docker](https://www.docker.com/)
- [AWS CLI](https://aws.amazon.com/cli/)
- [LocalStack](https://localstack.cloud/)
- [Go](https://golang.org/) (for local development)

---

## Running the Service Locally

### **1. Clone the Repository**

```bash
git clone git@github.com:NikolaySav/aws-examples-dynamodb-s3.git
cd aws-examples-dynamodb-s3
```

### **2. Build and Start the Services**

```bash
docker-compose up --build
```

### **3. Create a Bucket and Table**

```bash
aws --endpoint-url=http://localhost:4566 s3 mb s3://file-storage-bucket --region us-east-1

aws --endpoint-url=http://localhost:4566 dynamodb create-table \
    --table-name file-storage-table \
    --attribute-definitions \
        AttributeName=ID,AttributeType=S \
        AttributeName=Hash,AttributeType=S \
    --key-schema \
        AttributeName=ID,KeyType=HASH \
    --global-secondary-indexes \
        "[{\"IndexName\": \"HashIndex\", \"KeySchema\": [{\"AttributeName\": \"Hash\", \"KeyType\": \"HASH\"}], \"Projection\": {\"ProjectionType\": \"ALL\"}, \"ProvisionedThroughput\": {\"ReadCapacityUnits\": 1, \"WriteCapacityUnits\": 1}}]" \
    --provisioned-throughput ReadCapacityUnits=1,WriteCapacityUnits=1
```

## Query examples

### **1. Upload a File**

```bash
POST http://localhost:8080/file
Content-Type: multipart/form-data; boundary=WebAppBoundary
Accept: application/json

--WebAppBoundary
Content-Disposition: form-data; name="file"; filename="2024-10-07 16.39.46.jpg"
Content-Type: image/jpeg

< /Users/user/Downloads/2024-10-07 16.39.46.jpg
--WebAppBoundary--
```

```json
{
  "metadata": {
    "id": "17f6c3d2-4415-46ec-a70c-741127b73c20",
    "hash": "a3e8d378cfce4471a34ebbd744eae7029e83e4ead7472af8258ae3a06bae6278",
    "extension": ".jpg",
    "created_at": "2024-11-27T12:25:35Z",
    "updated_at": "2024-11-27T12:25:35Z"
  },
  "presigned_url": "http://localstack:4566/file-storage-bucket/17f6c3d2-4415-46ec-a70c-741127b73c20.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=test%2F20241127%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20241127T122541Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host&X-Amz-Signature=596e445f7b995fb3f942c1aedc4fcda1f084eb8089ec48cf8d7de22f4a6254ea"
}
```

### **2. Get File Metadata**

```bash
GET http://localhost:8080/file/17f6c3d2-4415-46ec-a70c-741127b73c20
Accept: application/json
Content-Type: application/json
```

```json
{
  "metadata": {
    "id": "d4d021a1-f9d9-437c-88c4-559eb7d69cca",
    "hash": "a3e8d378cfce4471a34ebbd744eae7029e83e4ead7472af8258ae3a06bae6278",
    "extension": ".jpg",
    "created_at": "2024-11-27T12:25:09Z",
    "updated_at": "2024-11-27T12:25:09Z"
  },
  "presigned_url": "http://localstack:4566/file-storage-bucket/d4d021a1-f9d9-437c-88c4-559eb7d69cca.jpg?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=test%2F20241127%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20241127T122519Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host&X-Amz-Signature=51fa2dc2db812ec528213376abd5690a901c296fd6263b5d7423b9ea0efde515"
}
```

### **3. Delete a File**

```bash
DELETE http://localhost:8080/file/d4d021a1-f9d9-437c-88c4-559eb7d69cca
Accept: application/json
```
