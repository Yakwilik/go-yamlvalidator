// Package genruntime contains the runtime ABI used by yamlvalidator-gen output.
//
// Application code normally does not call this package directly. Generated
// files use it for typed YAML encoding/decoding, mixed-tree fallbacks, decode
// limits, merge handling, and cycle detection.
package genruntime
