# Contributing

MemAuthority accepts changes that preserve the documented compatibility contract for the affected release line.

Before submitting code changes, run:

```sh
go test -count=1 ./...
go vet ./...
go mod verify
bash tools/check-docs.sh
```

Documentation changes should keep intentional English/Chinese guide pairs aligned, resolve repository-local Markdown links, and leave the runnable example Vault valid.

For compatibility-sensitive changes, update the next versioned contract rather than rewriting an already released contract in place. Breaking public changes require a major version; backward-compatible public additions use a minor version; compatible fixes use a patch version.
