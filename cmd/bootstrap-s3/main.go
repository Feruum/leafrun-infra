// bootstrap-s3 configures the dedicated development bucket before Storage starts.
package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"log"
	"os"
	"time"
)

func main() {
	if os.Getenv("LEAFRUN_DEV_FIXTURE") != "true" {
		log.Fatal("explicit LEAFRUN_DEV_FIXTURE=true required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(os.Getenv("LEAFRUN_STORAGE_S3_ENDPOINT"))
		o.UsePathStyle = true
	})
	bucket := os.Getenv("LEAFRUN_STORAGE_S3_BUCKET")
	if bucket != "leafrun-storage" && bucket != "leafrun-storage-test" && bucket != "leafrun-e06-storage" {
		log.Fatal("dedicated development bucket required")
	}
	buckets := []string{"leafrun-storage", "leafrun-storage-test", "leafrun-storage-regression"}
	if bucket == "leafrun-e06-storage" {
		buckets = []string{"leafrun-e06-storage", "leafrun-e06-storage-test"}
	}
	for _, bucket = range buckets {
		var lastObservedError error
		for {
			if _, err = client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &bucket}); err == nil {
				break
			}
			if ctx.Err() == nil || lastObservedError == nil {
				lastObservedError = err
			}
			select {
			case <-ctx.Done():
				log.Fatal("development bucket did not become ready; ", readinessDiagnostic(lastObservedError))
			case <-time.After(time.Second):
			}
		}
		_, err = client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: &bucket, VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}})
		if err != nil {
			log.Fatal(err)
		}
		out, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &bucket})
		if err != nil || out.Status != types.BucketVersioningStatusEnabled {
			log.Fatal("bucket versioning unavailable")
		}
	}
	fmt.Println("Separate stand and test bucket versioning enabled")
}
