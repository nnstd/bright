# Bright

A high-performance full-text search database with REST API built with Go, Fiber, and Bleve.

## Features

- 🔍 Full-text search with powerful query syntax
- 📊 Multiple index management
- 🚀 High-performance
- 🔄 Sorting and pagination support
- 🎯 Attribute filtering (include/exclude)
- 💾 Persistent storage with automatic index recovery

## Running

### Using Docker

```bash
# Run with authentication
docker run -p 3000:3000 -e BRIGHT_MASTER_KEY="your-secret-key" -v bright-data:/root/data ghcr.io/nnstd/bright:latest
```

The server will be on `http://localhost:3000`

### Using Helm (Kubernetes)

```bash
helm repo add bright https://nnstd.github.io/bright
helm install bright bright/bright
```

## Testing

Run the included test script:

```bash
./test-api.sh
```

## Query Syntax

Bleve supports powerful query syntax:

- Simple term: `hello`
- Phrase: `"hello world"`
- Field-specific: `title:hello`
- Boolean: `hello AND world`, `hello OR world`, `NOT spam`
- Wildcards: `hel*`, `he?lo`
- Fuzzy: `hello~2`
- Range: `age:>10`, `date:[2020-01-01 TO 2020-12-31]`

See [Bleve Query String documentation](https://blevesearch.com/docs/Query-String-Query/) for more details.
