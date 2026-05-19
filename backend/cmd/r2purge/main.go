// r2purge bulk-deletes objects from the configured S3-compatible bucket
// (typically R2). It defaults to dry-run; pass --confirm to actually delete.
//
// Intended as an ops-only tool. Reads bucket / endpoint / credentials from
// the same config + .env that cmd/api uses, so it cannot accidentally target
// a different account.
//
// Usage:
//
//	go run ./cmd/r2purge --dry-run                  # default, lists what would be deleted
//	go run ./cmd/r2purge --confirm                  # actually delete everything in the bucket
//	go run ./cmd/r2purge --prefix=ab/ --confirm     # restrict to a key prefix
//	go run ./cmd/r2purge --confirm --batch-size=500 # tune delete batch size (max 1000)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/logger"
)

func main() {
	prefix := flag.String("prefix", "", "Key prefix filter (empty = whole bucket)")
	dryRun := flag.Bool("dry-run", true, "If true, only list what would be deleted; default true for safety")
	confirm := flag.Bool("confirm", false, "Set to true to actually delete (overrides --dry-run)")
	batchSize := flag.Int("batch-size", 1000, "DeleteObjects batch size (1..1000)")
	configPath := flag.String("config", "", "Config file path; defaults to CONFIG_PATH or ./configs/config.yaml")
	sampleN := flag.Int("sample", 5, "Number of sample keys to print at the start")
	flag.Parse()

	config.LoadDotEnv()
	appLogger := logger.NewServiceFromEnv("emomo-r2purge")
	logger.SetDefaultLogger(appLogger)
	defer logger.Sync()

	if *batchSize < 1 || *batchSize > 1000 {
		appLogger.WithField("batch_size", *batchSize).Fatal("Invalid batch size; must be 1..1000")
	}

	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = os.Getenv("CONFIG_PATH")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		appLogger.WithError(err).Fatal("Failed to load config")
	}
	sc := cfg.GetStorageConfig()

	if sc.Bucket == "" {
		appLogger.Fatal("storage.bucket is empty; refusing to operate")
	}

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

	mode := "DRY-RUN"
	if *confirm {
		mode = "DELETE"
	}
	fmt.Printf("[r2purge] mode=%s bucket=%s endpoint=%s prefix=%q batch=%d\n",
		mode, sc.Bucket, endpointURL, *prefix, *batchSize)

	ctx := context.Background()

	var (
		continuationToken *string
		totalListed       int64
		totalDeleted      int64
		totalErrors       int64
		batch             []s3types.ObjectIdentifier
		printedSamples    int
	)

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		if *dryRun && !*confirm {
			batch = batch[:0]
			return
		}
		out, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(sc.Bucket),
			Delete: &s3types.Delete{
				Objects: batch,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			appLogger.WithError(err).WithFields(logger.Fields{
				"listed":          totalListed,
				"already_deleted": totalDeleted,
			}).Fatal("DeleteObjects failed")
		}
		if out != nil {
			totalDeleted += int64(len(batch) - len(out.Errors))
			for _, e := range out.Errors {
				totalErrors++
				fmt.Fprintf(os.Stderr, "  delete-err key=%s code=%s msg=%s\n",
					aws.ToString(e.Key), aws.ToString(e.Code), aws.ToString(e.Message))
			}
		} else {
			totalDeleted += int64(len(batch))
		}
		fmt.Printf("[r2purge] deleted_so_far=%d listed_so_far=%d errors=%d\n",
			totalDeleted, totalListed, totalErrors)
		batch = batch[:0]
	}

	for {
		out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(sc.Bucket),
			Prefix:            aws.String(*prefix),
			ContinuationToken: continuationToken,
			MaxKeys:           aws.Int32(int32(*batchSize)),
		})
		if err != nil {
			appLogger.WithError(err).WithField("listed", totalListed).Fatal("ListObjectsV2 failed")
		}

		for _, obj := range out.Contents {
			totalListed++
			if printedSamples < *sampleN {
				fmt.Printf("  sample key=%s size=%d\n",
					aws.ToString(obj.Key), aws.ToInt64(obj.Size))
				printedSamples++
			}
			batch = append(batch, s3types.ObjectIdentifier{Key: obj.Key})
			if len(batch) >= *batchSize {
				flushBatch()
			}
		}

		if !aws.ToBool(out.IsTruncated) {
			break
		}
		continuationToken = out.NextContinuationToken
	}

	flushBatch()

	if *dryRun && !*confirm {
		fmt.Printf("[r2purge] DRY-RUN done. Would delete %d objects. Re-run with --confirm to actually delete.\n", totalListed)
		return
	}
	fmt.Printf("[r2purge] done. listed=%d deleted=%d errors=%d\n", totalListed, totalDeleted, totalErrors)
	if totalErrors > 0 {
		os.Exit(1)
	}
}
