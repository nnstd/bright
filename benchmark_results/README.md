# Benchmark Results

This directory contains performance benchmark results comparing Bright with Meilisearch.

## Running Benchmarks

To run benchmarks locally:

```bash
chmod +x benchmark.sh
./benchmark.sh
```

## CI Benchmarks

Benchmarks run automatically on every commit to main/master branch and on pull requests.

Results are posted as:
- Comments on pull requests
- Comments on commits
- Artifacts in GitHub Actions

## Test Data

Benchmark datasets are generated using:

```bash
go run benchmarks/generate_data.go
```

This creates:
- `test_data_1000.jsonl` - 1,000 documents
- `test_data_5000.jsonl` - 5,000 documents
- `test_data_10000.jsonl` - 10,000 documents

## Metrics

The benchmark measures:
- **Indexing time**: Time to index all documents
- **Search time**: Average response time for various queries

## Comparison

Results are compared against [Meilisearch v1.31.0](https://github.com/meilisearch/meilisearch/releases/tag/v1.31.0).
