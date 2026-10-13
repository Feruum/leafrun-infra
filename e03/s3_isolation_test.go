package e03

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestS3CatalogIsolation(t *testing.T) {
	if os.Getenv("LEAFRUN_E03_INTEGRATION") != "1" {
		t.Skip("requires separate stand/test S3 roles")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	endpoint := os.Getenv("LEAFRUN_STORAGE_S3_ENDPOINT")
	makeClient := func(key, secret string) *s3.Client {
		t.Helper()
		if key == "" || secret == "" {
			t.Fatal("missing isolated S3 role")
		}
		cfg, err := config.LoadDefaultConfig(ctx, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(key, secret, "")))
		if err != nil {
			t.Fatal(err)
		}
		return s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true })
	}
	stand := makeClient(os.Getenv("LEAFRUN_E03_STAND_S3_ACCESS_KEY"), os.Getenv("LEAFRUN_E03_STAND_S3_SECRET_KEY"))
	test := makeClient(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"))
	for _, pair := range []struct {
		bucket       string
		owner, other *s3.Client
	}{{"leafrun-storage", stand, test}, {"leafrun-storage-test", test, stand}} {
		versioning, err := pair.owner.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &pair.bucket})
		if err != nil || versioning.Status != "Enabled" {
			t.Fatal("own bucket versioning unavailable", err)
		}
		key := fmt.Sprintf("role-isolation/%d", time.Now().UnixNano())
		written, err := pair.owner.PutObject(ctx, &s3.PutObjectInput{Bucket: &pair.bucket, Key: &key, Body: strings.NewReader("isolated catalog canary")})
		if err != nil || aws.ToString(written.VersionId) == "" {
			t.Fatal("own role cannot write a version", err)
		}
		t.Cleanup(func() {
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			_, err := pair.owner.DeleteObject(cleanup, &s3.DeleteObjectInput{Bucket: &pair.bucket, Key: &key, VersionId: written.VersionId})
			if err != nil {
				t.Error("canary cleanup", err)
			}
		})
		out, err := pair.other.GetObject(ctx, &s3.GetObjectInput{Bucket: &pair.bucket, Key: &key, VersionId: written.VersionId})
		if out != nil {
			_ = out.Body.Close()
		}
		var response *smithyhttp.ResponseError
		if !errors.As(err, &response) || response.HTTPStatusCode() != 403 {
			t.Fatalf("other catalog role read private version: %v", err)
		}
		_, err = pair.other.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: &pair.bucket})
		if !errors.As(err, &response) || response.HTTPStatusCode() != 403 {
			t.Fatalf("other catalog inventory accessed private bucket: %v", err)
		}
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/"+pair.bucket+"/"+key, nil)
		if err != nil {
			t.Fatal(err)
		}
		anonymous, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, anonymous.Body)
		_ = anonymous.Body.Close()
		if anonymous.StatusCode != 403 {
			t.Fatal("anonymous private object access", anonymous.StatusCode)
		}
	}
}
