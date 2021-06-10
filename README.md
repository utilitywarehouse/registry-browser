# registry-browser

Depending on the storage driver you're using, listing repositories and tags via
the docker registry API can be incredibly slow and resource intensive when you're
storing a large number of objects. This makes implementing a performant UI for the
registry using the API impractical.

This project provides a responsive UI by listing objects directly from the
storage backend and only calling the registry to retrieve manifest information.

Storage drivers currently supported:
- S3

## Deployment

A Kustomize base for deploying the browser can be found at
[manifests/base](manifests/base).

Refer to the [example](manifests/example).
