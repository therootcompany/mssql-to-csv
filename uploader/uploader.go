package uploader

import (
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// Auth is S3 credentials
type Auth struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// Uploader uploads a file to an s3 location
type Uploader struct {
	client *s3.S3
}

// New creates an s3 client as uploader
func New(auth Auth) (*Uploader, error) {
	creds := credentials.NewStaticCredentials(
		auth.AccessKeyID,
		auth.SecretAccessKey,
		"",
	)
	cfg := aws.NewConfig().WithRegion(auth.Region).WithCredentials(creds)

	return &Uploader{
		client: s3.New(session.New(), cfg),
	}, nil
}

// Upload will PUT the s3 body
func (u *Uploader) Upload(bucket, key string, body io.ReadSeeker) error {
	_, err := u.client.PutObject(&s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   body,
	})

	return err
}
