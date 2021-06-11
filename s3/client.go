package s3

import (
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

const (
	// The path within the bucket under which repositories are stored
	repositoriesPrefix = "docker/registry/v2/repositories"
	// This is the delimiter used in the s3 bucket to separate elements of a
	// given object name. Facilitates listing the common prefixes 'under' a
	// given path as if it were a filesystem.
	delimiter = "/"
)

// Client lists registry objects in an S3 bucket
type Client struct {
	bucket string
	svc    s3iface.S3API
}

// New returns a new client
func New(bucket string) (*Client, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	svc := s3.New(sess, aws.NewConfig())

	return &Client{
		bucket: bucket,
		svc:    svc,
	}, nil
}

// List returns the base names of the items directly under a particular prefix
func (c *Client) List(prefix string) ([]string, error) {
	absPrefix := filepath.Join(repositoriesPrefix, prefix) + "/"

	resp, err := c.svc.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket:    aws.String(c.bucket),
		Prefix:    aws.String(absPrefix),
		Delimiter: aws.String(delimiter),
	})
	if err != nil {
		return []string{}, err
	}

	var items []string
	for _, p := range resp.CommonPrefixes {
		items = append(items, filepath.Base(*p.Prefix))
	}

	return items, nil
}
