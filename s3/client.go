package s3

import (
	"path/filepath"
	"time"

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

type S3ObjectInfo struct {
	Key          string
	LastModified time.Time
	Size         int64
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

	input := &s3.ListObjectsV2Input{
		Bucket:    aws.String(c.bucket),
		Prefix:    aws.String(absPrefix),
		Delimiter: aws.String(delimiter),
	}

	var items []string

	err := c.svc.ListObjectsV2Pages(input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, p := range page.CommonPrefixes {
			items = append(items, filepath.Base(*p.Prefix))
		}
		return !lastPage
	})
	if err != nil {
		return []string{}, err
	}

	return items, nil
}

// ListTagObjectsWithMetadata returns metadata for all S3 objects under the given prefix
// that end with "/current/link", which represent active tag pointers in the registry.
func (c *Client) ListTagObjectsWithMetadata(prefix string) ([]S3ObjectInfo, error) {
	var results []S3ObjectInfo

	absPrefix := filepath.Join(repositoriesPrefix, prefix) + "/"

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(absPrefix),
	}

	err := c.svc.ListObjectsV2Pages(input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			results = append(results, S3ObjectInfo{
				Key:          *obj.Key,
				LastModified: *obj.LastModified,
				Size:         *obj.Size,
			})
		}
		return !lastPage
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}
