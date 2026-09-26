package recipes_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestAllValidationEntryPoints(t *testing.T) {
	validator := v.NewValidator(&v.FieldSchema{Type: v.TypeString})
	data := []byte("api")
	opts := v.ValidationContext{MaxDocuments: 1}
	calls := []struct {
		name string
		run  func() (*v.ValidationResult, error)
	}{
		{"bytes", func() (*v.ValidationResult, error) { return validator.ValidateBytes(data), nil }},
		{"options", func() (*v.ValidationResult, error) { return validator.ValidateWithOptions(data, opts), nil }},
		{"context", func() (*v.ValidationResult, error) { return validator.ValidateContext(context.Background(), data) }},
		{"context options", func() (*v.ValidationResult, error) {
			return validator.ValidateContextWithOptions(context.Background(), data, opts)
		}},
		{"reader", func() (*v.ValidationResult, error) { return validator.ValidateReader(strings.NewReader(string(data))) }},
		{"reader options", func() (*v.ValidationResult, error) {
			return validator.ValidateReaderWithOptions(strings.NewReader(string(data)), opts)
		}},
		{"reader context", func() (*v.ValidationResult, error) {
			return validator.ValidateReaderContext(context.Background(), strings.NewReader(string(data)))
		}},
		{"reader context options", func() (*v.ValidationResult, error) {
			return validator.ValidateReaderContextWithOptions(context.Background(), strings.NewReader(string(data)), opts)
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.run()
			if err != nil {
				t.Fatal(err)
			}
			if result.Canceled || result.Truncated || result.HasErrors() {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type countedReader struct {
	reader io.Reader
	count  int
}

func (r *countedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += n
	return n, err
}

func TestIncompleteAndIOResults(t *testing.T) {
	validator := v.NewValidator(&v.FieldSchema{Type: v.TypeAny})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := validator.ValidateContext(ctx, []byte("api"))
	if !errors.Is(err, context.Canceled) || result == nil || !result.Canceled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	result, err = validator.ValidateReader(failingReader{})
	if !errors.Is(err, io.ErrUnexpectedEOF) || result != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	reader := &countedReader{reader: strings.NewReader(strings.Repeat("x", 100))}
	result, err = validator.ValidateReaderWithOptions(reader, v.ValidationContext{MaxBytes: 8})
	if err != nil || !hasCode(result, "max_bytes") || reader.count > 9 {
		t.Fatalf("read=%d result=%+v err=%v", reader.count, result, err)
	}
	warningSchema := v.NewValidator(&v.FieldSchema{Type: v.TypeMap})
	result = warningSchema.ValidateWithOptions([]byte("a: x\nb: x\n"), v.ValidationContext{MaxDiagnostics: 1})
	if !result.Truncated || result.HasErrors() {
		t.Fatal("expect a truncated warning-only result, not complete success")
	}
}

func TestSnapshotAndConcurrentReuse(t *testing.T) {
	schema := stringFields("name")
	schema.AllowedKeys["name"].Required = true
	validator, err := v.CompileFieldSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	schema.AllowedKeys["name"].Type = v.TypeInt
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 10 {
				if result := validator.ValidateBytes([]byte("name: api")); result.HasErrors() {
					t.Errorf("snapshot changed: %v", result.Collector.Errors())
					return
				}
			}
		}()
	}
	group.Wait()
}
