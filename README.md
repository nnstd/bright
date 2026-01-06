# Bright

A high-performance full-text search database with REST API built with Go, Fiber, and Bleve.

## Features

- 🔍 Full-text search with powerful query syntax
- 📊 Multiple index management
- 🚀 High-performance
- 🔄 Sorting and pagination support
- 🎯 Attribute filtering (include/exclude)
- 💾 Persistent storage with automatic index recovery

## Installation

```bash
go mod download
```

## Running

```bash
go run main.go
# or
./search-db
```

The server will start on `http://localhost:3000`

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
