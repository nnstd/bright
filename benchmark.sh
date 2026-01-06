#!/bin/bash
set -e

# Benchmark script comparing Bright and Meilisearch
# Usage: ./benchmark.sh

MEILISEARCH_URL="https://github.com/meilisearch/meilisearch/releases/download/v1.31.0/meilisearch-linux-amd64"
MEILISEARCH_BIN="./meilisearch"
BRIGHT_BIN="./search-db"
BRIGHT_URL="http://localhost:3000"
MEILI_URL="http://localhost:7700"
RESULTS_DIR="benchmark_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Benchmark: Bright vs Meilisearch ===${NC}"

# Clean up function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    pkill -f meilisearch || true
    pkill -f search-db || true
    sleep 2
}

trap cleanup EXIT

# Create results directory
mkdir -p "$RESULTS_DIR"

# Download Meilisearch if not present
if [ ! -f "$MEILISEARCH_BIN" ]; then
    echo -e "${BLUE}Downloading Meilisearch...${NC}"
    curl -L "$MEILISEARCH_URL" -o "$MEILISEARCH_BIN"
    chmod +x "$MEILISEARCH_BIN"
fi

# Build Bright
echo -e "${BLUE}Building Bright...${NC}"
go build -o "$BRIGHT_BIN" .

# Generate test data
echo -e "${BLUE}Generating test data...${NC}"
go run benchmarks/generate_data.go

# Function to measure time
measure_time() {
    local start=$(date +%s%N)
    eval "$1"
    local end=$(date +%s%N)
    echo $(( (end - start) / 1000000 )) # Convert to milliseconds
}

# Function to test indexing
test_indexing() {
    local engine=$1
    local url=$2
    local index_name=$3
    local data_file=$4
    
    echo -e "${YELLOW}Testing indexing for $engine...${NC}"
    
    if [ "$engine" = "bright" ]; then
        # Create index
        curl -s -X POST "$url/indexes" \
            -H "Content-Type: application/json" \
            -d "{\"id\": \"$index_name\", \"primaryKey\": \"id\"}" > /dev/null
        
        # Index documents
        local time=$(measure_time "curl -s -X POST '$url/indexes/$index_name/documents?format=jsoneachrow' \
            -H 'Content-Type: application/json' \
            --data-binary @$data_file > /dev/null")
    else
        # Meilisearch
        curl -s -X POST "$url/indexes" \
            -H "Content-Type: application/json" \
            -d "{\"uid\": \"$index_name\", \"primaryKey\": \"id\"}" > /dev/null
        
        # Convert JSON Lines to JSON array for Meilisearch
        local json_array=$(cat $data_file | jq -s '.')
        
        local time=$(measure_time "curl -s -X POST '$url/indexes/$index_name/documents' \
            -H 'Content-Type: application/json' \
            -d '$json_array' > /dev/null")
    fi
    
    echo "$time"
}

# Function to test search
test_search() {
    local engine=$1
    local url=$2
    local index_name=$3
    local query=$4
    
    if [ "$engine" = "bright" ]; then
        local time=$(measure_time "curl -s -X POST '$url/indexes/$index_name/searches?q=$query' > /dev/null")
    else
        local time=$(measure_time "curl -s -X POST '$url/indexes/$index_name/search' \
            -H 'Content-Type: application/json' \
            -d '{\"q\": \"$query\"}' > /dev/null")
    fi
    
    echo "$time"
}

# Start Bright
echo -e "${BLUE}Starting Bright...${NC}"
"$BRIGHT_BIN" &
BRIGHT_PID=$!
sleep 3

# Wait for Bright to be ready
for i in {1..10}; do
    if curl -s "$BRIGHT_URL/indexes" > /dev/null 2>&1; then
        echo -e "${GREEN}Bright is ready${NC}"
        break
    fi
    echo "Waiting for Bright to start..."
    sleep 1
done

# Run Bright benchmarks
echo -e "${GREEN}=== Benchmarking Bright ===${NC}"
BRIGHT_INDEX_TIME=$(test_indexing "bright" "$BRIGHT_URL" "products" "benchmarks/test_data.jsonl")
echo -e "Indexing time: ${GREEN}${BRIGHT_INDEX_TIME}ms${NC}"

sleep 2

BRIGHT_SEARCH_1=$(test_search "bright" "$BRIGHT_URL" "products" "laptop")
BRIGHT_SEARCH_2=$(test_search "bright" "$BRIGHT_URL" "products" "computer")
BRIGHT_SEARCH_3=$(test_search "bright" "$BRIGHT_URL" "products" "price:>100")

echo -e "Search 'laptop': ${GREEN}${BRIGHT_SEARCH_1}ms${NC}"
echo -e "Search 'computer': ${GREEN}${BRIGHT_SEARCH_2}ms${NC}"
echo -e "Search 'price:>100': ${GREEN}${BRIGHT_SEARCH_3}ms${NC}"

# Stop Bright
kill $BRIGHT_PID
sleep 2

# Start Meilisearch
echo -e "${BLUE}Starting Meilisearch...${NC}"
"$MEILISEARCH_BIN" --no-analytics &
MEILI_PID=$!
sleep 5

# Wait for Meilisearch to be ready
for i in {1..10}; do
    if curl -s "$MEILI_URL/health" > /dev/null 2>&1; then
        echo -e "${GREEN}Meilisearch is ready${NC}"
        break
    fi
    echo "Waiting for Meilisearch to start..."
    sleep 1
done

# Run Meilisearch benchmarks
echo -e "${GREEN}=== Benchmarking Meilisearch ===${NC}"
MEILI_INDEX_TIME=$(test_indexing "meilisearch" "$MEILI_URL" "products" "benchmarks/test_data.jsonl")
echo -e "Indexing time: ${GREEN}${MEILI_INDEX_TIME}ms${NC}"

sleep 2

MEILI_SEARCH_1=$(test_search "meilisearch" "$MEILI_URL" "products" "laptop")
MEILI_SEARCH_2=$(test_search "meilisearch" "$MEILI_URL" "products" "computer")
MEILI_SEARCH_3=$(test_search "meilisearch" "$MEILI_URL" "products" "price")

echo -e "Search 'laptop': ${GREEN}${MEILI_SEARCH_1}ms${NC}"
echo -e "Search 'computer': ${GREEN}${MEILI_SEARCH_2}ms${NC}"
echo -e "Search 'price': ${GREEN}${MEILI_SEARCH_3}ms${NC}"

# Stop Meilisearch
kill $MEILI_PID

# Generate results
echo -e "${BLUE}=== Results ===${NC}"
RESULT_FILE="$RESULTS_DIR/benchmark_${TIMESTAMP}.json"

# Calculate averages
BRIGHT_AVG=$(( (BRIGHT_SEARCH_1 + BRIGHT_SEARCH_2 + BRIGHT_SEARCH_3) / 3 ))
MEILI_AVG=$(( (MEILI_SEARCH_1 + MEILI_SEARCH_2 + MEILI_SEARCH_3) / 3 ))

# Ensure all values are valid numbers, default to 0 if empty
BRIGHT_INDEX_TIME=${BRIGHT_INDEX_TIME:-0}
BRIGHT_SEARCH_1=${BRIGHT_SEARCH_1:-0}
BRIGHT_SEARCH_2=${BRIGHT_SEARCH_2:-0}
BRIGHT_SEARCH_3=${BRIGHT_SEARCH_3:-0}
MEILI_INDEX_TIME=${MEILI_INDEX_TIME:-0}
MEILI_SEARCH_1=${MEILI_SEARCH_1:-0}
MEILI_SEARCH_2=${MEILI_SEARCH_2:-0}
MEILI_SEARCH_3=${MEILI_SEARCH_3:-0}

cat > "$RESULT_FILE" <<EOF
{
  "timestamp": "$TIMESTAMP",
  "bright": {
    "indexing_ms": $BRIGHT_INDEX_TIME,
    "search_laptop_ms": $BRIGHT_SEARCH_1,
    "search_computer_ms": $BRIGHT_SEARCH_2,
    "search_complex_ms": $BRIGHT_SEARCH_3,
    "avg_search_ms": $BRIGHT_AVG
  },
  "meilisearch": {
    "indexing_ms": $MEILI_INDEX_TIME,
    "search_laptop_ms": $MEILI_SEARCH_1,
    "search_computer_ms": $MEILI_SEARCH_2,
    "search_complex_ms": $MEILI_SEARCH_3,
    "avg_search_ms": $MEILI_AVG
  }
}
EOF

echo -e "${GREEN}Results saved to: $RESULT_FILE${NC}"

# Display comparison
echo ""
echo -e "${BLUE}=== Comparison ===${NC}"
printf "%-20s %-15s %-15s %-15s\n" "Metric" "Bright" "Meilisearch" "Winner"
printf "%-20s %-15s %-15s %-15s\n" "--------------------" "---------------" "---------------" "---------------"

# Indexing comparison
if [ $BRIGHT_INDEX_TIME -lt $MEILI_INDEX_TIME ]; then
    WINNER="Bright"
else
    WINNER="Meilisearch"
fi
printf "%-20s %-15s %-15s %-15s\n" "Indexing (ms)" "$BRIGHT_INDEX_TIME" "$MEILI_INDEX_TIME" "$WINNER"

# Average search comparison
BRIGHT_AVG=$(( (BRIGHT_SEARCH_1 + BRIGHT_SEARCH_2 + BRIGHT_SEARCH_3) / 3 ))
MEILI_AVG=$(( (MEILI_SEARCH_1 + MEILI_SEARCH_2 + MEILI_SEARCH_3) / 3 ))

if [ $BRIGHT_AVG -lt $MEILI_AVG ]; then
    WINNER="Bright"
else
    WINNER="Meilisearch"
fi
printf "%-20s %-15s %-15s %-15s\n" "Avg Search (ms)" "$BRIGHT_AVG" "$MEILI_AVG" "$WINNER"

echo ""
echo -e "${GREEN}Benchmark complete!${NC}"
