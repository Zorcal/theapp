# Schemas

All API schemas are located in this folder for easy access and consistency.

## gRPC

For guidance on best practices, please refer to [Google's API Design
Patterns](https://cloud.google.com/apis/design/design_patterns).

Quicklist:

- Get: [AIP-131](https://google.aip.dev/131)
- List: [AIP-132](https://google.aip.dev/132)
  - Filtering: [AIP-160](https://google.aip.dev/160) — see note below
- Create: [AIP-133](https://google.aip.dev/133)
- Update: [AIP-134](https://google.aip.dev/134)
  - Field masks: [AIP-161](https://google.aip.dev/161)
- Delete: [AIP-135](https://google.aip.dev/135)

### Filtering

AIP-160 specifies a free-form filter string that supports arbitrary boolean expressions, wildcards, and traversal into nested fields. Full compliance requires a parser, an AST, and ongoing work as new operators are needed — complexity that is rarely justified for internal APIs with a small, known set of filter fields.

It is acceptable to deviate from AIP-160 and use typed filter messages instead: a dedicated protobuf message where each filterable field is an explicit, optional field.

Any deviation must be documented on the relevant List RPC with a comment that names the supported subset and explains why a typed filter was chosen over full AIP-160 compliance.

## Swagger / OpenAPI

`openapi/*.json` specs is generated from the proto schemas by `protoc-gen-openapiv2`. Do not edit it by hand — run `make generate` to regenerate it after changing any `.proto` file.

### HTTP headers

Swagger UI's "Try it out" only shows an input for a header if it's declared in the spec — a header nothing declares can't be sent from the UI at all. Declare one as an `openapiv2_operation.parameters.headers` entry:

```proto
option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_operation) = {
  parameters: {
    headers: {
      name: "x-project-id"
      description: "ID of the project this request operates in."
      type: INTEGER
      required: true
    }
  }
};
```

Add `x-project-id` to an RPC that operates within a project's scope. Add `x-idempotency-key` to an RPC whose handler calls a DBOS workflow:

```proto
option (grpc.gateway.protoc_gen_openapiv2.options.openapiv2_operation) = {
  parameters: {
    headers: {
      name: "x-idempotency-key"
      description: "A client-generated UUID. Retrying with the same key returns the original result instead of repeating the operation."
      type: STRING
      required: false
    }
  }
};
```
