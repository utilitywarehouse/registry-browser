package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/utilitywarehouse/registry-browser/registry"
	"github.com/utilitywarehouse/registry-browser/s3"
)

var (
	flagS3Bucket      = flag.String("s3-bucket", defaultFromEnv("REGISTRY_BROWSER_S3_BUCKET", ""), "S3 bucket name")
	flagListenAddress = flag.String("listen-address", defaultFromEnv("REGISTRY_BROWSER_LISTEN_ADDRESS", ":8080"), "Listen address")
	flagRegistryURL   = flag.String("registry-url", defaultFromEnv("REGISTRY_BROWSER_REGISTRY_URL", "http://localhost:5000"), "Registry url")
)

func defaultFromEnv(key, defaultValue string) string {
	v := os.Getenv(key)
	if len(v) == 0 {
		return defaultValue
	}

	return v
}

func main() {
	flag.Parse()

	if *flagS3Bucket == "" {
		log.Fatalln("Must provide an s3 bucket")
	}

	if *flagRegistryURL == "" {
		log.Fatalln("Must provide a registry URL")
	}

	registryClient, err := registry.New(*flagRegistryURL)
	if err != nil {
		log.Fatalln(err)
	}

	s3Client, err := s3.New(*flagS3Bucket)
	if err != nil {
		log.Fatalln(err)
	}

	s, err := newServer(registryClient, s3Client)
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("Listening on %s", *flagListenAddress)
	log.Fatalln(http.ListenAndServe(*flagListenAddress, s))
}
