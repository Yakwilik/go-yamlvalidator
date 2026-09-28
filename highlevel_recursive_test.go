package yamlvalidator_test

import (
	"strings"
	"testing"

	"github.com/Yakwilik/go-yamlvalidator"
)

type runtimeRecursiveNode struct {
	Name     string                 `yaml:"name" yamlvalidate:"required"`
	Children []runtimeRecursiveNode `yaml:"children,omitempty"`
}

type runtimeRecursiveLink struct {
	Value string                `yaml:"value"`
	Next  *runtimeRecursiveLink `yaml:"next,omitempty"`
}

func TestRuntimeRecursiveGoTypes(t *testing.T) {
	var node runtimeRecursiveNode
	if err := yamlvalidator.Unmarshal([]byte("name: root\nchildren: [{name: leaf}]\n"), &node); err != nil {
		t.Fatal(err)
	}
	if node.Children[0].Name != "leaf" {
		t.Fatalf("%+v", node)
	}
	if err := yamlvalidator.Unmarshal([]byte("name: root\nchildren: [{}]\n"), &node); err == nil {
		t.Fatal("nested required name accepted")
	}
	var link runtimeRecursiveLink
	if err := yamlvalidator.Unmarshal([]byte("value: root\nnext: {value: leaf}\n"), &link); err != nil {
		t.Fatal(err)
	}
	if link.Next == nil || link.Next.Value != "leaf" {
		t.Fatalf("%+v", link)
	}
	if raw, err := yamlvalidator.Marshal(link); err != nil || !strings.Contains(string(raw), "leaf") {
		t.Fatalf("%s %v", raw, err)
	}
	link.Next.Next = &link
	if _, err := yamlvalidator.Marshal(link); err == nil {
		t.Fatal("value cycle accepted")
	}
}
