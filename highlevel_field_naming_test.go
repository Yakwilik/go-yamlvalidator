package yamlvalidator_test

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

type fieldNamingWorker struct{ ImageName string }
type fieldNamingInline struct{ Region string }
type fieldNamingConfig struct {
	fieldNamingInline               `yaml:",inline"`
	SkipValidateSanPortsForceUPTask bool
	ImageName                       string `json:"image_name"`
	Host                            string `yaml:"server_host"`
	Optional                        string `yaml:",omitempty"`
	Flow                            []int  `yaml:",flow"`
	Secret                          string `yaml:"-"`
	private                         string
	Workers                         map[string]*fieldNamingWorker
	List                            []fieldNamingWorker
	Array                           [1]fieldNamingWorker
	Pointer                         *fieldNamingWorker
}

func TestFieldNamingGoRoundTrip(t *testing.T) {
	t.Parallel()
	input := []byte("Region: eu\nSkipValidateSanPortsForceUPTask: true\nImageName: service-image\nserver_host: api\nFlow: [1, 2]\nWorkers: {product_UP: {ImageName: upper}, product_up: {ImageName: lower}}\nList: [{ImageName: list}]\nArray: [{ImageName: array}]\nPointer: {ImageName: pointer}\n")
	for _, skip := range []bool{false, true} {
		t.Run(map[bool]string{false: "validated", true: "unchecked"}[skip], func(t *testing.T) {
			t.Parallel()
			var got fieldNamingConfig
			if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal(input, &got); err != nil {
				t.Fatal(err)
			}
			if !got.SkipValidateSanPortsForceUPTask || got.ImageName != "service-image" || got.Region != "eu" || got.Workers["product_UP"].ImageName != "upper" || got.Workers["product_up"].ImageName != "lower" {
				t.Fatalf("decoded: %#v", got)
			}
			got.private = "keep-private"
			data, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip, Indent: 2}).Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"ImageName:", "Region:", "server_host:", "Flow: [1, 2]"} {
				if !strings.Contains(string(data), key) {
					t.Fatalf("missing %s in %s", key, data)
				}
			}
			for _, key := range []string{"Optional:", "Secret:", "private:", "image_name:"} {
				if strings.Contains(string(data), key) {
					t.Fatalf("unexpected %s in %s", key, data)
				}
			}
			var roundtrip fieldNamingConfig
			if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal(data, &roundtrip); err != nil {
				t.Fatal(err)
			}
			if roundtrip.private != "" {
				t.Fatal("unexported field was decoded")
			}
			got.private = "" // Unexported state is deliberately absent from the roundtrip.
			if !reflect.DeepEqual(got, roundtrip) {
				t.Fatalf("roundtrip:\n%#v\n%#v", got, roundtrip)
			}
		})
	}
}

func TestFieldNamingPoliciesAndDiagnostics(t *testing.T) {
	t.Parallel()
	type config struct {
		FeatureEnabled bool
		ImageName      string `yamlvalidate:"minLength=3"`
	}
	for _, key := range []string{"featureenabled", "feature_enabled"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			for _, policy := range []v.UnknownKeyPolicy{v.UnknownKeyInherit, v.UnknownKeyError, v.UnknownKeyWarn, v.UnknownKeyIgnore} {
				got := config{ImageName: "original"}
				var diagnostics []v.ValidationError
				err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, UnknownKeyPolicy: policy, OnDiagnostic: func(d v.ValidationError) { diagnostics = append(diagnostics, d) }}).Unmarshal([]byte(key+": true\n"), &got)
				if policy == v.UnknownKeyInherit || policy == v.UnknownKeyError {
					if err == nil {
						t.Fatal("unknown key accepted")
					}
					continue
				}
				if err != nil || got.FeatureEnabled {
					t.Fatalf("policy %v: %#v %v", policy, got, err)
				}
				if policy == v.UnknownKeyWarn && len(diagnostics) != 1 {
					t.Fatalf("warnings: %v", diagnostics)
				}
			}
		})
	}
	got := config{ImageName: "original"}
	err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("FeatureEnabled: true\nImageName: x\n"), &got)
	var validation *v.ValidationErrors
	if !errors.As(err, &validation) {
		t.Fatalf("expected diagnostics: %v", err)
	}
	diagnostics := validation.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Path != "ImageName" || diagnostics[0].Line != 2 {
		t.Fatalf("diagnostics: %+v", diagnostics)
	}
	if got.FeatureEnabled || got.ImageName != "original" {
		t.Fatalf("validation mutated destination: %#v", got)
	}
}

func TestFieldNamingConflictsBeforeMutation(t *testing.T) {
	t.Parallel()
	type inline struct{ Value string }
	for _, destination := range []any{
		&struct {
			Value string
			Other string `yaml:"Value"`
		}{Value: "original"},
		&struct {
			Other string `yaml:"Value"`
			Value string
		}{Value: "original"},
		&struct {
			inline `yaml:",inline"`
			Other  string `yaml:"Value"`
		}{inline: inline{Value: "original"}},
	} {
		before := reflect.ValueOf(destination).Elem().Interface()
		for _, skip := range []bool{false, true} {
			err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal([]byte("Value: changed\n"), destination)
			if err == nil {
				t.Fatalf("conflict accepted: %T skip=%v", destination, skip)
			}
			if !reflect.DeepEqual(before, reflect.ValueOf(destination).Elem().Interface()) {
				t.Fatal("conflict mutated destination")
			}
			if _, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Marshal(destination); err == nil {
				t.Fatal("marshal conflict accepted")
			}
		}
	}
}

func TestFieldNamingRulesAndSpecialValues(t *testing.T) {
	t.Parallel()
	type config struct {
		Mode     string `yamlvalidate:"when={field=Mode,eq=prod,require=[Token]}"`
		Token    string
		Duration time.Duration
		Time     time.Time
		Bytes    []byte
		Node     yaml.Node
	}
	input := []byte("Mode: prod\nToken: secret\nDuration: 2s\nTime: 2026-01-02T03:04:05Z\nBytes: [1, 2]\nNode: {Keep_CASE: value}\n")
	var got config
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal(input, &got); err != nil {
		t.Fatal(err)
	}
	if got.Duration != 2*time.Second || !reflect.DeepEqual(got.Bytes, []byte{1, 2}) || got.Node.Content[0].Value != "Keep_CASE" {
		t.Fatalf("special values: %#v", got)
	}
	data, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo}).Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var again config
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal(data, &again); err != nil {
		t.Fatal(err)
	}
	if again.Duration != got.Duration || !again.Time.Equal(got.Time) || !reflect.DeepEqual(again.Bytes, got.Bytes) {
		t.Fatalf("special roundtrip: %#v", again)
	}
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("Mode: prod\n"), new(config)); err == nil {
		t.Fatal("conditional rule skipped")
	}
}

func TestFieldNamingCacheIsolation(t *testing.T) {
	t.Parallel()
	type config struct{ ImageName string }
	registry, err := v.NewRegistry(v.RegistryConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := func(naming v.FieldNaming) error {
		key := "imagename"
		if naming == v.FieldNamingGo {
			key = "ImageName"
		}
		var got config
		if err := (v.UnmarshalOptions{FieldNaming: naming, Registry: registry}).Unmarshal([]byte(key+": image\n"), &got); err != nil {
			return err
		}
		if got.ImageName != "image" {
			return errors.New("field was not decoded")
		}
		data, err := (v.MarshalOptions{FieldNaming: naming, Registry: registry}).Marshal(got)
		if err == nil && !strings.Contains(string(data), key+": image") {
			return errors.New("wrong encoded key")
		}
		return err
	}
	for _, naming := range []v.FieldNaming{v.FieldNamingYAML, v.FieldNamingGo, v.FieldNamingYAML} {
		if err := run(naming); err != nil {
			t.Fatal(err)
		}
	}
	var group sync.WaitGroup
	for i := range 24 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := run(v.FieldNaming(i % 2)); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
}

type fieldNamingOpaque struct{ Node *yaml.Node }

func (c *fieldNamingOpaque) UnmarshalYAML(node *yaml.Node) error { c.Node = node; return nil }
func (c fieldNamingOpaque) MarshalYAML() (any, error)            { return c.Node, nil }

type fieldNamingText string

func (c *fieldNamingText) UnmarshalText(data []byte) error {
	*c = fieldNamingText("read:" + string(data))
	return nil
}
func (c fieldNamingText) MarshalText() ([]byte, error) {
	return []byte(strings.TrimPrefix(string(c), "read:")), nil
}

type fieldNamingGuard struct{ Calls *int }

func (c *fieldNamingGuard) UnmarshalYAML(*yaml.Node) error { *c.Calls++; return nil }
func (c *fieldNamingGuard) MarshalYAML() (any, error)      { *c.Calls++; return map[string]string{}, nil }

func TestFieldNamingCustomCodecsAndInvalidOptions(t *testing.T) {
	t.Parallel()
	type config struct {
		Opaque fieldNamingOpaque `yamlvalidate:"type=any"`
		Text   fieldNamingText   `yamlvalidate:"type=any"`
	}
	for _, skip := range []bool{false, true} {
		input := []byte("Opaque: {Keep_CASE: value}\nText: raw\n")
		var got config
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal(input, &got); err != nil {
			t.Fatal(err)
		}
		if got.Text != "read:raw" || got.Opaque.Node.Content[0].Value != "Keep_CASE" || got.Opaque.Node.Line != 1 {
			t.Fatalf("codec input: %#v", got)
		}
		data, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Marshal(got)
		if err != nil || !strings.Contains(string(data), "Keep_CASE: value") || !strings.Contains(string(data), "Text: raw") {
			t.Fatalf("custom marshal: %s %v", data, err)
		}
	}
	for _, skip := range []bool{false, true} {
		calls := 0
		guard := fieldNamingGuard{Calls: &calls}
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNaming(42), SkipValidation: skip}).Unmarshal([]byte("{}"), &guard); err == nil {
			t.Fatal("invalid mode accepted")
		}
		if _, err := (v.MarshalOptions{FieldNaming: v.FieldNaming(42), SkipValidation: skip}).Marshal(&guard); err == nil {
			t.Fatal("invalid mode accepted")
		}
		if calls != 0 {
			t.Fatalf("invalid mode called codecs %d times", calls)
		}
	}
}

func TestFieldNamingNullMergeAndDuplicateKeys(t *testing.T) {
	t.Parallel()
	type child struct{ ImageName string }
	type config struct {
		Base  child `yaml:"base"`
		Child *child
		List  []child
		Map   map[string]child
		Value child
	}
	for _, skip := range []bool{false, true} {
		options := v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}
		var got config
		input := []byte("base: &base {ImageName: image}\nChild: {<<: *base}\nList: [*base]\nMap: {product_UP: *base}\n")
		if err := options.Unmarshal(input, &got); err != nil {
			t.Fatal(err)
		}
		if got.Child.ImageName != "image" || got.List[0].ImageName != "image" || got.Map["product_UP"].ImageName != "image" {
			t.Fatalf("aliases: %#v", got)
		}
		got.Value.ImageName = "kept"
		if err := options.Unmarshal([]byte("Child: null\nList: null\nMap: null\nValue: null\n"), &got); err != nil && skip {
			t.Fatal(err)
		}
		// A non-nullable struct is rejected during validation; unchecked null retains its value.
		if skip && (got.Child != nil || got.List != nil || got.Map != nil || got.Value.ImageName != "kept") {
			t.Fatalf("null: %#v", got)
		}
		if err := options.Unmarshal([]byte("Child: {ImageName: one, ImageName: two}\n"), &got); err == nil {
			t.Fatal("duplicate accepted")
		}
		if err := options.Unmarshal([]byte("Child: &loop {ImageName: *loop}\n"), &got); err == nil {
			t.Fatal("cyclic alias accepted")
		}
	}
}

func TestFieldNamingLiteralSchemaAndRegistry(t *testing.T) {
	t.Parallel()
	type bound struct{ ImageName string }
	literal := &v.FieldSchema{Type: v.TypeMap, AllowedKeys: map[string]*v.FieldSchema{"ImageName": {Type: v.TypeString, Required: true}}}
	registry, err := v.NewRegistry(v.RegistryConfig{TypeBindings: map[reflect.Type]v.TypeBinding{reflect.TypeFor[bound](): {Decode: literal, Encode: literal}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, naming := range []v.FieldNaming{v.FieldNamingGo, v.FieldNamingYAML, v.FieldNamingGo} {
		var got bound
		err := (v.UnmarshalOptions{FieldNaming: naming, Registry: registry, Schema: v.NewValidator(literal)}).Unmarshal([]byte("ImageName: image\n"), &got)
		if err != nil {
			t.Fatal(err)
		}
		expected := ""
		if naming == v.FieldNamingGo {
			expected = "image"
		}
		if got.ImageName != expected {
			t.Fatalf("mode %v: %#v", naming, got)
		}
	}
	type config struct{ ImageName string }
	external := v.NewValidator(&v.FieldSchema{Type: v.TypeMap, AllowedKeys: map[string]*v.FieldSchema{"imagename": {Type: v.TypeString, Required: true}}})
	err = (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, Schema: external}).Unmarshal([]byte("ImageName: image\n"), new(config))
	if err == nil {
		t.Fatal("external schema keys were renamed")
	}
}

type fieldNamingOldHook struct{ Value string }

func (c *fieldNamingOldHook) UnmarshalYAML(decode func(any) error) error {
	c.Value = "hook"
	return nil
}

type fieldNamingInlineHook struct{ Seen string }

func (c *fieldNamingInlineHook) UnmarshalYAML(node *yaml.Node) error {
	c.Seen = node.Content[0].Value
	return nil
}

func TestFieldNamingCodecCompatibilityRegressions(t *testing.T) {
	t.Parallel()
	t.Run("legacy hook", func(t *testing.T) {
		var got struct {
			Custom fieldNamingOldHook `yamlvalidate:"type=any"`
		}
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("Custom: {Value: input}\n"), &got); err != nil {
			t.Fatal(err)
		}
		if got.Custom.Value != "hook" {
			t.Fatalf("legacy hook bypassed: %#v", got)
		}
	})
	t.Run("inline hook", func(t *testing.T) {
		var got struct {
			Hook  fieldNamingInlineHook `yaml:",inline"`
			Other string
		}
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: true}).Unmarshal([]byte("Foreign_CASE: input\nOther: value\n"), &got); err != nil {
			t.Fatal(err)
		}
		if got.Hook.Seen != "Foreign_CASE" || got.Other != "value" {
			t.Fatalf("inline hook bypassed: %#v", got)
		}
	})
	t.Run("map null", func(t *testing.T) {
		type child struct{ ImageName string }
		got := map[string]child{"product_UP": {ImageName: "kept"}}
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("product_UP: null\n"), &got); err != nil { // Struct null is rejected by validation, so use the unchecked path.
			if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: true}).Unmarshal([]byte("product_UP: null\n"), &got); err != nil {
				t.Fatal(err)
			}
		}
		if got["product_UP"].ImageName != "kept" {
			t.Fatalf("null overwrote map value: %#v", got)
		}
	})
	t.Run("validated pointer depth", func(t *testing.T) {
		type child struct{ ImageName string }
		var got struct{ Child **child }
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, Limits: v.Limits{MaxDepth: 3}}).Unmarshal([]byte("Child: {ImageName: image}\n"), &got); err != nil {
			t.Fatal(err)
		}
		if (**got.Child).ImageName != "image" {
			t.Fatalf("%#v", got)
		}
	})
}

func TestFieldNamingZeroOptionsRemainStandard(t *testing.T) {
	t.Parallel()
	type config struct {
		ImageName string
		Flag      bool   `yaml:",omitempty"`
		Literal   string `yaml:"custom_KEY"`
	}
	input := []byte("imagename: image\ncustom_KEY: literal\n")
	for _, decode := range []func([]byte, any) error{v.Unmarshal, (v.UnmarshalOptions{}).Unmarshal} {
		var got config
		if err := decode(input, &got); err != nil {
			t.Fatal(err)
		}
		if got.ImageName != "image" || got.Literal != "literal" {
			t.Fatalf("default decode: %#v", got)
		}
		for _, encode := range []func(any) ([]byte, error){v.Marshal, (v.MarshalOptions{}).Marshal} {
			data, err := encode(got)
			if err != nil || !strings.Contains(string(data), "imagename: image") || strings.Contains(string(data), "ImageName:") {
				t.Fatalf("default encode: %s %v", data, err)
			}
		}
	}
}

type fieldNamingKey string

func (key *fieldNamingKey) UnmarshalText(data []byte) error {
	*key = fieldNamingKey("decoded:" + string(data))
	return nil
}
func (key fieldNamingKey) MarshalText() ([]byte, error) { return []byte("encoded:" + string(key)), nil }

func TestFieldNamingMapKeysAndNullSequences(t *testing.T) {
	t.Parallel()
	var got map[fieldNamingKey]fieldNamingWorker
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("product_UP: {ImageName: image}\n"), &got); err != nil {
		t.Fatal(err)
	}
	if got["decoded:product_UP"].ImageName != "image" {
		t.Fatalf("key hook skipped: %#v", got)
	}
	data, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo}).Marshal(map[fieldNamingKey]fieldNamingWorker{"product_UP": {ImageName: "image"}})
	if err != nil || !strings.Contains(string(data), "encoded:product_UP:") {
		t.Fatalf("key marshal: %s %v", data, err)
	}
	existing := map[string]int{"existing": 5}
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: true}).Unmarshal([]byte("missing: null\nexisting: null\n"), &existing); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(existing, map[string]int{"existing": 5, "missing": 0}) {
		t.Fatalf("map null: %#v", existing)
	}
	var list []int
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: true}).Unmarshal([]byte("[1, null, 2]"), &list); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(list, []int{1, 2}) {
		t.Fatalf("null sequence: %#v", list)
	}
	array := [3]int{9, 8, 7}
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: true}).Unmarshal([]byte("[1, null, 2]"), &array); err != nil {
		t.Fatal(err)
	}
	if array != [3]int{1, 2, 7} {
		t.Fatalf("null array: %#v", array)
	}
}

type fieldNamingYAMLKey string

func (key *fieldNamingYAMLKey) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		*key = fieldNamingYAMLKey(node.Content[0].Value)
		return nil
	}
	*key = fieldNamingYAMLKey(node.Value)
	return nil
}

func TestFieldNamingCustomComplexKeyUnchecked(t *testing.T) {
	t.Parallel()
	var got map[fieldNamingYAMLKey]fieldNamingWorker
	input := []byte("? {Keep_CASE: key}\n: {ImageName: image}\n")
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: true}).Unmarshal(input, &got); err != nil {
		t.Fatal(err)
	}
	if got["Keep_CASE"].ImageName != "image" {
		t.Fatalf("custom key node: %#v", got)
	}
}

func TestFieldNamingSkippedNullsRespectVisitLimits(t *testing.T) {
	t.Parallel()
	var got []int
	err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: true, Limits: v.Limits{MaxNodeVisits: 2}}).Unmarshal([]byte("[null, null]"), &got)
	if err == nil {
		t.Fatal("skipped nulls bypassed the visit limit")
	}
}
