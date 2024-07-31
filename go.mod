module github.com/utilitywarehouse/registry-browser

go 1.20

require (
	github.com/aws/aws-sdk-go v1.55.5
	github.com/distribution/distribution/v3 v3.0.0-beta.1
	github.com/distribution/reference v0.6.0
	github.com/gorilla/mux v1.8.1
	github.com/opencontainers/go-digest v1.0.0
)

require (
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.22.0 // indirect
)

// https://github.com/distribution/distribution/pull/3804
replace github.com/distribution/distribution/v3 => github.com/distribution/distribution/v3 v3.0.0-20230223072852-e5d5810851d1
