package plugin

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/pkg/errors"

	"github.com/meltwater/drone-s3-cache/cache"
	"github.com/meltwater/drone-s3-cache/provider"
)

// Plugin for caching directories using given Providers
type Plugin struct {
	// Indicates the files ACL, which should be one
	// of the following:
	//     private
	//     public-read
	//     public-read-write
	//     authenticated-read
	//     bucket-owner-read
	//     bucket-owner-full-control
	ACL     string
	Branch  string
	Bucket  string
	Default string // default master branch
	// if not "", enable server-side encryption
	// valid values are:
	//     AES256
	//     aws:kms
	Encryption string
	Endpoint   string
	Key        string
	Mount      []string
	// Use path style instead of domain style
	// Should be true for minio and false for AWS
	PathStyle bool
	Rebuild   bool
	Region    string
	Repo      string
	Restore   bool
	Secret    string
}

// Exec entry point of Plugin, where the magic happens
func (p *Plugin) Exec() error {
	conf := &aws.Config{
		Region:   aws.String(p.Region),
		Endpoint: &p.Endpoint,
		// TODO: Check any consequences?
		// DisableSSL:       aws.Bool(strings.HasPrefix(p.Endpoint, "http://")),
		DisableSSL:       aws.Bool(!strings.HasPrefix(p.Endpoint, "https://")),
		S3ForcePathStyle: aws.Bool(p.PathStyle),
	}

	// allowing to use the instance role or provide a key and secret
	if p.Key != "" && p.Secret != "" {
		conf.Credentials = credentials.NewStaticCredentials(p.Key, p.Secret, "")
	}
	// TODO: Else return and error
	// TODO: Check if both (rebuild, restore) of them set.

	cacheProvider := provider.NewS3(p.Bucket, p.ACL, p.Encryption, conf)

	if p.Rebuild {
		if err := p.processRebuild(cacheProvider); err != nil {
			return errors.Wrap(err, "process rebuild failed")
		}
	}

	if p.Restore {
		if err := p.processRestore(cacheProvider); err != nil {
			return errors.Wrap(err, "process restore failed")
		}
	}

	return nil
}

// Helpers

// processRebuild the remote cache from the local environment
func (p Plugin) processRebuild(c cache.Provider) error {
	now := time.Now()
	for _, mount := range p.Mount {
		cacheKey := hash(mount, p.Branch)
		path := filepath.Join(p.Repo, cacheKey)

		log.Printf("archiving directory <%s> to remote cache <%s>", mount, path)
		if err := cache.Upload(c, mount, path); err != nil {
			return errors.Wrap(err, "could not upload")
		}
	}
	log.Printf("cache built in %v", time.Since(now))
	return nil
}

// processRestore the local environment from the remote cache
func (p Plugin) processRestore(c cache.Provider) error {
	now := time.Now()
	for _, mount := range p.Mount {
		cacheKey := hash(mount, p.Branch)
		path := filepath.Join(p.Repo, cacheKey)

		log.Printf("restoring directory <%s> from remote cache <%s>", mount, path)
		if err := cache.Download(c, path, mount); err != nil {
			return errors.Wrap(err, "could not download")
		}
	}
	log.Printf("cache restored in %v", time.Since(now))
	return nil
}

// hash a file name based on path and branch
func hash(mount, branch string) string {
	parts := []string{mount, branch}

	// calculate the hash using the branch
	h := md5.New()
	for _, part := range parts {
		io.WriteString(h, part)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
