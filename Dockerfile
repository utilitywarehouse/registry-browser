FROM golang:1.22-alpine AS build
WORKDIR /go/src/github.com/utilitywarehouse/registry-browser
COPY . /go/src/github.com/utilitywarehouse/registry-browser
ENV CGO_ENABLED 0
RUN apk --no-cache add git \
      && go get -t ./... \
      && go test ./... \
      && go build -o /registry-browser .

FROM alpine:3.20
ADD templates /templates
ADD static /static
COPY --from=build /registry-browser /registry-browser

ENTRYPOINT [ "/registry-browser" ]
