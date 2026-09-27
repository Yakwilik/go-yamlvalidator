package yamlvalidator

import (
	"sync"
	"testing"
)

func TestHighLevelConcurrentOptionsAndRegistries(t *testing.T) {
	type config struct {
		Name string `yaml:"name" yamlvalidate:"required,minLength=2"`
	}
	a, err := NewRegistry(RegistryConfig{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewRegistry(RegistryConfig{})
	if err != nil {
		t.Fatal(err)
	}
	options := []UnmarshalOptions{{Registry: a}, {Registry: b}}
	var group sync.WaitGroup
	errors := make(chan error, 64)
	for i := 0; i < 64; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			var out config
			err := options[i%2].Unmarshal([]byte("name: safe\n"), &out)
			if err == nil && out.Name != "safe" {
				err = &SchemaError{Reason: "concurrent decode lost value"}
			}
			errors <- err
		}(i)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}
