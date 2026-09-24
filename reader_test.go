package yamlvalidator_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
)

func TestValidateReader(t *testing.T) {
	validator, err := CompileFieldSchema(&FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {Type: TypeString, Required: true},
		},
		UnknownKeyPolicy: UnknownKeyError,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := validator.ValidateReader(strings.NewReader(`name: value
`))
	if err != nil {
		t.Fatalf("validate reader: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("valid reader input rejected: %v", result.Collector.Errors())
	}
}

func TestValidateReaderHonorsMaxBytesWithoutReadingWholeStream(t *testing.T) {
	reader := &countingReader{reader: strings.NewReader(strings.Repeat("x", 1024))}
	validator := NewValidator(&FieldSchema{Type: TypeAny})

	result, err := validator.ValidateReaderWithOptions(reader, ValidationContext{MaxBytes: 16})
	if err != nil {
		t.Fatalf("validate reader: %v", err)
	}
	if !containsDiagnosticCode(result, "max_bytes") {
		t.Fatalf("expected max_bytes diagnostic, got %v", result.Collector.Errors())
	}
	if errors := result.Collector.Errors(); len(errors) != 1 || errors[0].Got != "> 16 bytes" {
		t.Fatalf("reader size diagnostic must not claim an exact truncated size: %v", errors)
	}
	if reader.read > 17 {
		t.Fatalf("reader consumed %d bytes, want at most MaxBytes+1", reader.read)
	}
}

func TestValidateReaderReturnsIOError(t *testing.T) {
	validator := NewValidator(&FieldSchema{Type: TypeAny})
	result, err := validator.ValidateReader(errorReader{})
	if err == nil || !strings.Contains(err.Error(), "read YAML input") {
		t.Fatalf("expected read error, got result=%v err=%v", result, err)
	}
	if result != nil {
		t.Fatalf("I/O failure should not return validation result: %+v", result)
	}
}

func TestValidateReaderContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterFirstRead{cancel: cancel}
	validator := NewValidator(&FieldSchema{Type: TypeAny})

	result, err := validator.ValidateReaderContext(ctx, reader)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if result == nil || !result.Canceled || !errors.Is(result.ContextErr, context.Canceled) {
		t.Fatalf("cancellation not represented in result: %+v", result)
	}
}

func TestValidateReaderRejectsNilReader(t *testing.T) {
	validator := NewValidator(&FieldSchema{Type: TypeAny})
	if _, err := validator.ValidateReader(nil); err == nil {
		t.Fatal("expected nil reader error")
	}
}

type countingReader struct {
	reader io.Reader
	read   int
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	n, err := reader.reader.Read(buffer)
	reader.read += n
	return n, err
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

type cancelAfterFirstRead struct {
	cancel context.CancelFunc
	read   bool
}

func (reader *cancelAfterFirstRead) Read(buffer []byte) (int, error) {
	if reader.read {
		return 0, errors.New("underlying reader should not be called after cancellation")
	}
	reader.read = true
	if len(buffer) == 0 {
		return 0, nil
	}
	buffer[0] = 'x'
	reader.cancel()
	return 1, nil
}
