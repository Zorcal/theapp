# backend

## Local debugging

The gRPC server registers reflection when `THEAPP_ENVIRONMENT=local`, so these commands work without a `buf`-built descriptor set:

```
grpcurl -plaintext 127.0.0.1:5051 list
grpcurl -plaintext -d '{}' 127.0.0.1:5051 theapp.v1.<service>/<rpc>
```

For example, `theapp.v1.UserService/ListUsers`.

Reflection stays off elsewhere to avoid exposing the schema.
