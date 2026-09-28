# Benchmarks

The benchmark suite compares four public paths and the underlying node codec:

- yaml.v3 with an ordinary Go struct: baseline serialization without yamlvalidator validation.
- yamlvalidator with an ordinary tagged struct: runtime schema path.
- yamlvalidator with the generated twin: generated codec plus the same high-level validation contract.
- yaml.v3 with the generated twin: standard YAML hooks supplied by yamlvalidator-gen.
- Node codec benchmarks: unchecked yaml.Node mapping only, isolating the part code generation actually replaces.

The generated and runtime models have the same fields and yaml/yamlvalidate rules. Workloads use 1 service × 2 endpoints, 20 × 4, and 100 × 5.

Generate the checked-in benchmark model with:

~~~sh
go generate ./benchmarks/model
~~~

Run the reproducible suite with:

~~~sh
go test -mod=readonly -run '^$' \
  -bench '^Benchmark(Unmarshal|Marshal|NodeCodec)$' \
  -benchmem -benchtime=1s -count=5 ./benchmarks
~~~

Use medians across the five runs when comparing results. Do not compare the yaml.v3 runtime baseline as if it had the same semantics: it does no yamlvalidate validation.

See results/2026-09-28-m4-max.md for the first recorded run.
