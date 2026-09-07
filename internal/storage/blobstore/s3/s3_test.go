package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/swayamsb50/web_crawler/internal/storage/blobstore"
)

// fakeAPI is an in-memory stand-in for the S3 client, keyed by bucket+key.
type fakeAPI struct {
	objects map[string][]byte
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{objects: make(map[string][]byte)}
}

func (f *fakeAPI) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	f.objects[*params.Bucket+"/"+*params.Key] = data
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeAPI) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	data, ok := f.objects[*params.Bucket+"/"+*params.Key]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func TestPutAndGetRoundTrip(t *testing.T) {
	store := &Store{client: newFakeAPI(), bucket: "test-bucket", prefix: ""}
	ctx := context.Background()

	if err := store.Put(ctx, "abc123", []byte("hello world")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	got, err := store.Get(ctx, "abc123")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("Get() = %q, want hello world", got)
	}
}

func TestGetMissingKeyReturnsErrNotFound(t *testing.T) {
	store := &Store{client: newFakeAPI(), bucket: "test-bucket", prefix: ""}
	_, err := store.Get(context.Background(), "does-not-exist")
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestKeyPrefixIsApplied(t *testing.T) {
	fake := newFakeAPI()
	store := &Store{client: fake, bucket: "test-bucket", prefix: "pages/"}
	ctx := context.Background()

	if err := store.Put(ctx, "abc123", []byte("data")); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if _, ok := fake.objects["test-bucket/pages/abc123"]; !ok {
		t.Errorf("expected object to be stored under prefixed key pages/abc123, got keys: %v", keys(fake.objects))
	}

	got, err := store.Get(ctx, "abc123")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got) != "data" {
		t.Errorf("Get() = %q, want data", got)
	}
}

func TestNewRejectsEmptyBucket(t *testing.T) {
	_, err := New(context.Background(), Config{Bucket: ""})
	if err == nil {
		t.Errorf("expected error for empty bucket")
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
