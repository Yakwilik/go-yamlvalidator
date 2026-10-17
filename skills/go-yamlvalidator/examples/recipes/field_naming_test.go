package recipes

import (
	"reflect"
	"strings"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestHighLevelGoFieldNames(t *testing.T) {
	t.Parallel()
	type worker struct{ ImageName string }
	type config struct {
		SkipValidateSanPortsForceUPTask bool
		ImageName                       string `json:"image_name"`
		Host                            string `yaml:"server_host"`
		Optional                        string `yaml:",omitempty"`
		Workers                         map[string]*worker
	}
	input := []byte("SkipValidateSanPortsForceUPTask: true\nImageName: task-image\nserver_host: api\nWorkers: {product_UP: {ImageName: upper}, product_up: {ImageName: lower}}\n")
	for _, skip := range []bool{false, true} {
		var got config
		options := v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, WarningsAsErrors: true, SkipValidation: skip}
		if err := options.Unmarshal(input, &got); err != nil {
			t.Fatal(err)
		}
		if !got.SkipValidateSanPortsForceUPTask || got.ImageName != "task-image" || got.Workers["product_UP"].ImageName != "upper" || got.Workers["product_up"].ImageName != "lower" {
			t.Fatalf("unexpected decode: %#v", got)
		}
		data, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip, Indent: 2}).Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "ImageName: task-image") || !strings.Contains(string(data), "server_host: api") || strings.Contains(string(data), "Optional:") {
			t.Fatalf("unexpected YAML: %s", data)
		}
		var roundtrip config
		if err := options.Unmarshal(data, &roundtrip); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, roundtrip) {
			t.Fatalf("roundtrip changed config: %#v", roundtrip)
		}
	}
	var got config
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("imagename: wrong-case\n"), &got); err == nil {
		t.Fatal("wrong-case key accepted")
	}
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, UnknownKeyPolicy: v.UnknownKeyIgnore}).Unmarshal([]byte("imagename: ignored\n"), &got); err != nil || got.ImageName != "" {
		t.Fatalf("unknown key became an alias: %#v %v", got, err)
	}
}
