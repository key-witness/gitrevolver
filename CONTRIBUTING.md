# Contributing

GitRevolver is a Go CLI. Keep identity routing process-local and do not introduce global provider auth switching.

Before opening a change:

```bash
make test
make vet
gitleaks detect --source . --verbose
```

Use fake provider IDs and fake tokens in examples and test fixtures. Keep upstream MIT attribution intact.
