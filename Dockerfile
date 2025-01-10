FROM golang:1.23-alpine AS build

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY . .

RUN go test -v -cover ./... \
      && CGO_ENABLED=0 go build -o /registry-browser .

FROM alpine:3.20
ADD templates /templates
ADD static /static
COPY --from=build /registry-browser /registry-browser

ENTRYPOINT [ "/registry-browser" ]
