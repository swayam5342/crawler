// Package blobstore defines a small key-value interface for storing raw
// page bodies, decoupled from the structured metadata store (which points
// at blobs by key). Implementations here cover local disk (default) and
// S3 (production), and either can be swapped in via config.
package blobstore

import "context"

// Store puts and gets opaque byte blobs by key.
type Store interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

// ErrNotFound is returned by Get when key does not exist.
var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "blobstore: key not found" }
