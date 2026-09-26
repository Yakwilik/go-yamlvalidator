# Native FieldSchema

Source: validator.go, schema_compile.go and their tests at the revision in
[api-index.md](api-index.md). Executable cases: [native tests](../examples/recipes/native_test.go).

## Structure and types

| Field or type | Behavior |
| --- | --- |
| TypeAny | Bare schema accepts values of any type without recursive field constraints; global syntax/alias/duplicate checks still run |
| TypeNull | Null only |
| TypeString | YAML string, including quoted strings |
| TypeInt | Integer; large integer literals retain exact classification |
| TypeFloat | Floating point or integer |
| TypeBool | Boolean; YAML 1.1 compatibility is opt-in in the library |
| TypeMap | Mapping; missing AllowedKeys does not mean arbitrary keys are accepted silently |
| TypeSequence | Sequence; ItemSchema constrains each item |
| AllowedTypes | Nonempty list takes precedence over Type; prefer concrete types, not TypeAny in the list |
| Required | Presence required at the parent mapping; a required root also rejects an entirely empty stream |
| Nullable | Allows null at the base type check; attached value validators still execute |
| Description | Human-readable metadata |
| Deprecated | Nonempty value emits warning when present; "true" selects a generic message |
| Default | Missing optional field emits a warning; no insertion into YAML or a Go struct |

Omitted key, explicit null, empty string, empty map and empty sequence are distinct.
Use <code>NonEmptyValidator</code>, length constraints or <code>MinItems</code> for
non-emptiness. A required nullable field may be present with null.

### Mapping policies

~~~go
schema := &v.FieldSchema{
    Type: v.TypeMap,
    UnknownKeyPolicy: v.UnknownKeyError,
    AllowedKeys: map[string]*v.FieldSchema{
        "name": {Type: v.TypeString, Required: true},
        "labels": {
            Type: v.TypeMap,
            AdditionalProperties: &v.FieldSchema{Type: v.TypeString},
        },
        "workers": {
            Type: v.TypeSequence,
            MinItems: v.Ptr(1),
            MaxItems: v.Ptr(8),
            ItemSchema: &v.FieldSchema{Type: v.TypeString},
        },
    },
}
~~~

This fragment needs the root package import aliased v. AdditionalProperties
validates every key not in AllowedKeys and takes precedence over unknown-key
policy. KeyValidators run for all effective mapping keys, including known keys.
Policies are local to each FieldSchema; setting error at the root does not set
nested schemas to error automatically.

| UnknownKeyPolicy | Result without AdditionalProperties |
| --- | --- |
| UnknownKeyInherit (zero value) | StrictKeys false: warning; StrictKeys true: error |
| UnknownKeyError | Error |
| UnknownKeyWarn | Warning |
| UnknownKeyIgnore | No unknown-key diagnostic |

For arbitrary unvalidated map values, use AdditionalProperties with TypeAny.
For a wholly unstructured value, use a bare TypeAny. Do not weaken a user's
schema merely to make a failing config pass.

## Relationships among fields

Define relationships on the parent mapping, and declare referenced fields in
AllowedKeys when using definition validation. The rules check presence, so null
still counts as present (its own field schema can then reject null).

| Rule | Meaning |
| --- | --- |
| AnyOf [][]string | At least one complete field group exists |
| ExactlyOneOf []string | Exactly one listed field exists |
| MutuallyExclusive []string | At most one listed field exists; zero is valid |
| OneOfRequired [][]string | Exactly one group is complete |
| ForbiddenTogether [][]string | Each fully present forbidden group is rejected |
| DependentRequired map[string][]string | Trigger presence requires every listed dependency |
| Conditions []ConditionalRule | Compare ConditionField's scalar text to ConditionValue; apply ThenRequired and ThenForbidden |

OneOfRequired alone is not a complete authentication policy. For example, token
plus username still has only one complete group if password is missing. The
recipe below rejects such mixtures as well:

~~~go
schema := &v.FieldSchema{
    Type: v.TypeMap,
    AllowedKeys: map[string]*v.FieldSchema{
        "token": {Type: v.TypeString},
        "username": {Type: v.TypeString},
        "password": {Type: v.TypeString},
    },
    UnknownKeyPolicy: v.UnknownKeyError,
    OneOfRequired: [][]string{{"token"}, {"username", "password"}},
    ForbiddenTogether: [][]string{{"token", "username"}, {"token", "password"}},
    DependentRequired: map[string][]string{
        "username": {"password"},
        "password": {"username"},
    },
}
~~~

ConditionValue is a string, not a predicate or Go bool. To test enabled, use
<code>ConditionValue: "true"</code>. The comparison is against scalar text;
quoting or enabling boolean compatibility does not turn it into typed equality.
Use a custom validator for a richer condition.

## Schema alternatives

<code>OneOfSchemas</code> requires exactly one successful schema;
<code>AnyOfSchemas</code> requires at least one. They are different from the
presence rules AnyOf and OneOfRequired. Alternatives run before the base schema
constraints, which must also pass. Branch warnings do not make a branch fail.
For disjoint branches, set types and unknown-key policies explicitly.

~~~go
schema := &v.FieldSchema{
    Type: v.TypeAny,
    OneOfSchemas: []*v.FieldSchema{
        {Type: v.TypeString},
        {Type: v.TypeSequence, ItemSchema: &v.FieldSchema{Type: v.TypeString}},
    },
}
~~~

Use JSON Schema for recursive schema graphs: CompileFieldSchema rejects native
recursive graphs. Sharing the same child schema in several places is allowed.

## Definition checking and snapshot

~~~go
if err := v.ValidateFieldSchema(schema); err != nil {
    return err
}
~~~

Alternatively, request both definition checking and a snapshot:

~~~go
validator, err := v.CompileFieldSchema(schema)
if err != nil {
    return err
}
~~~

Use either the standalone check or snapshot constructor, not both routinely.
Validation catches nil schemas/validators, invalid types, duplicate allowed
types, impossible item bounds, map/sequence constraints on incompatible types,
invalid field references and validators implementing DefinitionValidator.
Snapshots include nested schemas, common defaults, slices, maps and built-in
validator configuration. Opaque values and non-cloning custom validators need
caller-managed concurrency; see [native validators](native-validators.md).
