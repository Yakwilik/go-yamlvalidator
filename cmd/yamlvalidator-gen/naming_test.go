package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateNamingAwareCodecs(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(wd))
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module namingfixture\n\ngo 1.24.0\n\nrequire (\ngithub.com/Yakwilik/go-yamlvalidator v0.0.0\ngopkg.in/yaml.v3 v3.0.1\n)\nreplace github.com/Yakwilik/go-yamlvalidator => "+root+"\n")
	write("types.go", `package namingfixture
 import model "github.com/Yakwilik/go-yamlvalidator/examples/codegen/model"
 import "strings"
 import "time"
 import "gopkg.in/yaml.v3"
 type Child struct { ImageName string; Explicit string `+"`yaml:\"literal\"`"+` }
 type Inline struct { InlineValue string }
 type Config struct {
   Name string
   Optional string `+"`yaml:\",omitempty\"`"+`
   Child *Child
   Children []Child
   Array [1]Child
   Workers map[string]*Child
   Inline Inline `+"`yaml:\",inline\"`"+`
   Extra map[string]string `+"`yaml:\",inline\"`"+`
   Hidden string `+"`yaml:\"-\"`"+`
 }
 type GoGroups struct { Mode string `+"`yamlvalidate:\"when={field=Mode,eq=prod,require=[Token]}\"`"+`; Token string }
 type GoCollision struct { Field string; Explicit string `+"`yaml:\"Field\"`"+` }
 type MarshalOnly struct { Field string; Explicit string `+"`yaml:\"Field\"`"+` }
 func (MarshalOnly) MarshalYAML() (any,error) { return "owned",nil }
 type Directional struct { Before string; Child MarshalOnly }
 type External struct { Before string; Child model.GoNamingCollision }
 type RecursiveBoundary struct { Child RecursiveCustom }
 type RecursiveCustom struct { Next *RecursiveBoundary }
 type MapKey string
 func (key MapKey) MarshalText() ([]byte,error) { return []byte("wire_"+string(key)),nil }
 func (key *MapKey) UnmarshalText(value []byte) error { *key=MapKey(strings.TrimPrefix(string(value),"wire_"));return nil }
 type KeyConfig struct { Values map[MapKey]Child }
 type NullCustom struct { Value string }
 func (value *NullCustom) UnmarshalYAML(node *yaml.Node) error { if node.Tag=="!!null"{panic("custom codec received null")};value.Value=node.Value;return nil }
 type NullSequences struct {
  Duration time.Duration `+"`yaml:\"duration\"`"+`
  Ints []int `+"`yaml:\"ints\"`"+`
  IntArray [3]int `+"`yaml:\"int_array\"`"+`
  Strings []string `+"`yaml:\"strings\"`"+`
  Children []Child `+"`yaml:\"children\"`"+`
  Pointers []*Child `+"`yaml:\"pointers\"`"+`
  Maps []map[string]int `+"`yaml:\"maps\"`"+`
  Slices [][]int `+"`yaml:\"slices\"`"+`
  Interfaces []any `+"`yaml:\"interfaces\"`"+`
  Nodes []yaml.Node `+"`yaml:\"nodes\"`"+`
  Customs []NullCustom `+"`yaml:\"customs\"`"+`
 }
 func (RecursiveCustom) MarshalYAML() (any,error) { return "owned",nil }
 type YAMLCollision struct { ID string; Id string }
 type NestedCollision struct { Earlier string; Children []GoCollision }
 type InlineCollision struct { Field string; InlineAlias `+"`yaml:\",inline\"`"+` }
 type InlineAlias struct { Explicit string `+"`yaml:\"Field\"`"+` }
 `)
	cmd := exec.Command("go", "run", "github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen", "-all", "-output=zz_generated.go")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generator: %v\n%s", err, out)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "zz_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"YAMLValidatorEncodeWithNaming", "YAMLValidatorSchemaSpecWithNaming", "YAMLValidatorValidateFieldNaming", "YAMLValidatorFieldNamesWithNaming", "YAMLValidatorValidateFieldNamingWithContext"} {
		if !strings.Contains(string(generated), method) {
			t.Fatalf("missing naming-aware generated method %s", method)
		}
	}
	write("types_test.go", `package namingfixture
 import (
  "strings"
  "reflect"
  "testing"
  v "github.com/Yakwilik/go-yamlvalidator"
  "github.com/Yakwilik/go-yamlvalidator/genruntime"
  "gopkg.in/yaml.v3"
 )
 func TestNaming(t *testing.T) {
  original:=Config{Name:"root",Child:&Child{ImageName:"child",Explicit:"explicit"},Children:[]Child{{ImageName:"slice"}},Array:[1]Child{{ImageName:"array"}},Workers:map[string]*Child{"product_UP":{ImageName:"map"}},Inline:Inline{InlineValue:"inline"},Extra:map[string]string{"map_KEY":"extra"}}
  for _,naming:=range []genruntime.FieldNaming{genruntime.FieldNamingYAML,genruntime.FieldNamingGo,genruntime.FieldNamingYAML}{
   node,err:=original.YAMLValidatorEncodeWithNaming(naming);if err!=nil{t.Fatal(err)}
   ctx:=genruntime.NewDecodeContext(genruntime.Limits{},false);ctx.FieldNaming=naming
   var got Config;if err:=got.YAMLValidatorDecodeWithContext(node,ctx);err!=nil{t.Fatal(err)}
   if !reflect.DeepEqual(got,original){t.Fatalf("mode %d: got %+v, want %+v",naming,got,original)}
   key:=genruntime.FieldName("Name","",naming)
   if node.Content[0].Value!=key{t.Fatalf("mode %d: key %q, want %q",naming,node.Content[0].Value,key)}
   graph,_:=original.YAMLValidatorSchemaSpecWithNaming(naming);if graph.Error!=""{t.Fatal(graph.Error)}
   names:=original.YAMLValidatorFieldNamesWithNaming(naming)
   expectedNames:=[]string{genruntime.FieldName("Name","",naming),genruntime.FieldName("Optional","",naming),genruntime.FieldName("Child","",naming),genruntime.FieldName("Children","",naming),genruntime.FieldName("Array","",naming),genruntime.FieldName("Workers","",naming),genruntime.FieldName("InlineValue","",naming)}
   if !reflect.DeepEqual(names,expectedNames){t.Fatalf("field names: got %v, want %v",names,expectedNames)}
   if _,ok:=graph.Nodes[graph.Root].AllowedKeys[key];!ok{t.Fatalf("schema missing %q",key)}
  }
  node,err:=original.YAMLValidatorEncode();if err!=nil{t.Fatal(err)};if node.Content[0].Value!="name"{t.Fatal("default hook changed naming")}
  var exact Config;var document yaml.Node;if err:=yaml.Unmarshal([]byte("name: ignored\n"),&document);err!=nil{t.Fatal(err)}
  ctx:=genruntime.NewDecodeContext(genruntime.Limits{},false);ctx.FieldNaming=genruntime.FieldNamingGo
  if err:=exact.YAMLValidatorDecodeWithContext(document.Content[0],ctx);err!=nil{t.Fatal(err)}
  if exact.Name!="" || exact.Extra["name"]!="ignored"{t.Fatalf("wrong-case key bound to struct: %+v",exact)}
 }
 func TestModeSpecificGroups(t *testing.T){
  options:=v.UnmarshalOptions{FieldNaming:v.FieldNamingGo}
  var got GoGroups
  if err:=options.Unmarshal([]byte("Mode: prod\nToken: secret\n"),&got);err!=nil{t.Fatal(err)}
  if got.Mode!="prod" || got.Token!="secret"{t.Fatalf("%+v",got)}
  if err:=options.Unmarshal([]byte("Mode: prod\n"),&got);err==nil{t.Fatal("literal Go group reference not validated")}
  if err:=v.Unmarshal([]byte("mode: prod\ntoken: secret\n"),&got);err==nil{t.Fatal("YAML mode accepted Go-only group definition")}
  if err:=(v.UnmarshalOptions{FieldNaming:v.FieldNamingYAML,SkipValidation:true}).Unmarshal([]byte("mode: dev\n"),&got);err!=nil{t.Fatal(err)}
 }
 func TestDirectionalPreflight(t *testing.T){
  if err:=(Directional{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingGo,true);err!=nil{t.Fatal(err)}
  if err:=(Directional{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingGo);err==nil{t.Fatal("decode conflict hidden by marshal-only hook")}
  for _,skip:=range []bool{false,true}{
   value:=Directional{Before:"original"}
   err:=(v.UnmarshalOptions{FieldNaming:v.FieldNamingGo,SkipValidation:skip}).Unmarshal([]byte("Before: changed\nChild: {Field: first}\n"),&value)
   if err==nil || value.Before!="original"{t.Fatalf("decode conflict mutated destination, skip=%v: %+v %v",skip,value,err)}
   external:=External{Before:"original"}
   err=(v.UnmarshalOptions{FieldNaming:v.FieldNamingGo,SkipValidation:skip}).Unmarshal([]byte("Before: changed\nChild: {Field: first}\n"),&external)
   if err==nil || external.Before!="original"{t.Fatalf("external generated conflict mutated destination, skip=%v: %+v %v",skip,external,err)}
  }
 }
 func TestGeneratedMapKeyCodecs(t *testing.T){
  for _,naming:=range []v.FieldNaming{v.FieldNamingYAML,v.FieldNamingGo}{
   original:=KeyConfig{Values:map[MapKey]Child{"service":{Explicit:"child"}}}
   data,err:=(v.MarshalOptions{FieldNaming:naming,SkipValidation:true}).Marshal(original);if err!=nil{t.Fatal(err)}
   if !strings.Contains(string(data),"wire_service:"){t.Fatalf("key MarshalText bypassed: %s",data)}
   var value KeyConfig
   if err:=(v.UnmarshalOptions{FieldNaming:naming,SkipValidation:true}).Unmarshal(data,&value);err!=nil{t.Fatal(err)}
   if !reflect.DeepEqual(value,original){t.Fatalf("key codec roundtrip: got %+v, want %+v",value,original)}
  }
 }
 func TestGeneratedNullSequences(t *testing.T){
  source:=[]byte("duration: 1s\nints: [1, &nil null, *nil, 2]\nint_array: [1, null, 2]\nstrings: [one, null, two]\nchildren: [null, {literal: child}]\npointers: [null, {literal: child}]\nmaps: [null, {key: 1}]\nslices: [null, [1]]\ninterfaces: [null, text]\nnodes: [null, text]\ncustoms: [null, owned]\n")
  type reference NullSequences
  var expected reference
  if err:=yaml.Unmarshal(source,&expected);err!=nil{t.Fatal(err)}
  for _,naming:=range []v.FieldNaming{v.FieldNamingYAML,v.FieldNamingGo}{
   var value NullSequences
   if err:=(v.UnmarshalOptions{FieldNaming:naming,SkipValidation:true}).Unmarshal(source,&value);err!=nil{t.Fatal(err)}
   if !reflect.DeepEqual(value,NullSequences(expected)){t.Fatalf("mode %d: got %+v, want %+v",naming,value,expected)}
  }
  var document yaml.Node
  if err:=yaml.Unmarshal([]byte("ints: [null]\n"),&document);err!=nil{t.Fatal(err)}
  ctx:=genruntime.NewDecodeContext(genruntime.Limits{MaxNodeVisits:3},true);ctx.FieldNaming=genruntime.FieldNamingGo
  var limited NullSequences
  if err:=limited.YAMLValidatorDecodeWithContext(document.Content[0],ctx);err==nil{t.Fatal("null elements bypassed decode visit limits")}
 }
 func TestRecursivePreflight(t *testing.T){
  if err:=(RecursiveBoundary{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingGo);err!=nil{t.Fatal(err)}
  for _,skip:=range []bool{false,true}{
   var value RecursiveBoundary
   err:=(v.UnmarshalOptions{FieldNaming:v.FieldNamingGo,SkipValidation:skip}).Unmarshal([]byte("Child: {Next: null}\n"),&value)
   if err!=nil{t.Fatalf("recursive fallback, skip=%v: %v",skip,err)}
  }
 }
 func TestModeSpecificCollisions(t *testing.T){
  if err:=(GoCollision{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingYAML);err!=nil{t.Fatal(err)}
  if err:=(GoCollision{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingGo);err==nil{t.Fatal("Go collision accepted")}
  if err:=(YAMLCollision{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingGo);err!=nil{t.Fatal(err)}
  if err:=(YAMLCollision{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingYAML);err==nil{t.Fatal("YAML collision accepted")}
  if err:=(NestedCollision{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingGo);err==nil{t.Fatal("nested collision accepted")}
  if err:=(InlineCollision{}).YAMLValidatorValidateFieldNaming(genruntime.FieldNamingGo);err==nil{t.Fatal("inline collision accepted")}
  for _,skip:=range []bool{false,true}{
   original:=NestedCollision{Earlier:"original"}
   err:=(v.UnmarshalOptions{FieldNaming:v.FieldNamingGo,SkipValidation:skip}).Unmarshal([]byte("Earlier: changed\nChildren: []\n"),&original)
   if err==nil || original.Earlier!="original"{t.Fatalf("collision mutated destination, skip=%v: %+v %v",skip,original,err)}
  }
 }
 `)
	testContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd = exec.CommandContext(testContext, "go", "test", "-mod=mod", "-timeout=5s", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}
	cmd = exec.Command("go", "run", "github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen", "-all", "-output=zz_generated.go", "-check")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("freshness: %v\n%s", err, out)
	}
}
