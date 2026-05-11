// r2list lists the first N keys in the configured R2 bucket. Used during
// reembed debugging to verify which storage_key shape actually exists on R2.
package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
)

func main() {
	prefix := flag.String("prefix", "", "Key prefix filter (empty = top-level)")
	limit := flag.Int("limit", 20, "Max keys to list")
	configPath := flag.String("config", "", "Config path")
	flag.Parse()

	config.LoadDotEnv()
	appLogger := logger.NewServiceFromEnv("emomo-r2list")
	logger.SetDefaultLogger(appLogger)
	defer logger.Sync()

	cfg, err := config.Load(*configPath)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to load config")
	}
	sc := cfg.GetStorageConfig()

	endpoint := sc.Endpoint
	for _, p := range []string{"https://", "http://"} {
		endpoint = strings.TrimPrefix(endpoint, p)
	}
	endpoint = strings.SplitN(endpoint, "/", 2)[0]
	scheme := "http"
	if sc.UseSSL || strings.Contains(sc.Endpoint, "https") {
		scheme = "https"
	}
	endpointURL := fmt.Sprintf("%s://%s", scheme, endpoint)

	region := sc.Region
	if region == "" {
		region = "auto"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(sc.AccessKey, sc.SecretKey, "")),
	)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to load AWS config")
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
		o.UsePathStyle = true
	})

	fmt.Printf("bucket=%s endpoint=%s public_url=%s\n", sc.Bucket, endpointURL, sc.PublicURL)

	out, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket:  aws.String(sc.Bucket),
		Prefix:  aws.String(*prefix),
		MaxKeys: aws.Int32(int32(*limit)),
	})
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to list objects")
	}
	fmt.Printf("KeyCount=%d truncated=%v\n", aws.ToInt32(out.KeyCount), aws.ToBool(out.IsTruncated))
	for _, obj := range out.Contents {
		fmt.Printf("  %s  (%d bytes)\n", aws.ToString(obj.Key), aws.ToInt64(aws.Int64(*obj.Size)))
	}
}
