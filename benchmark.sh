#!/bin/bash
set -e

# Benchmark script comparing Bright and Meilisearch
# Usage: ./benchmark.sh

MEILISEARCH_URL="https://github.com/meilisearch/meilisearch/releases/download/v1.33.1/meilisearch-linux-amd64"
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
    # Kill our specific processes if they're still running
    if [ ! -z "$BRIGHT_PID" ] && kill -0 "$BRIGHT_PID" 2>/dev/null; then
        kill "$BRIGHT_PID" 2>/dev/null || true
    fi
    if [ ! -z "$MEILI_PID" ] && kill -0 "$MEILI_PID" 2>/dev/null; then
        kill "$MEILI_PID" 2>/dev/null || true
    fi
    # Cleanup any leftover processes (only user's processes, ignore errors)
    pkill -u "$USER" -f meilisearch 2>/dev/null || true
    pkill -u "$USER" -f search-db 2>/dev/null || true
    sleep 1
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

# Generate test data
echo -e "${BLUE}Generating test data...${NC}"
go run benchmarks/generate_data.go

# Build Bright
echo -e "${BLUE}Building Bright...${NC}"
go build -o "$BRIGHT_BIN" .

# Function to get memory usage in MB
get_memory_usage() {
    local pid=$1
    # Get RSS (Resident Set Size) in KB and convert to MB
    local mem_kb=$(ps -o rss= -p "$pid" 2>/dev/null | tr -d ' ')
    if [ -z "$mem_kb" ]; then
        echo "0"
    else
        echo $(( mem_kb / 1024 ))
    fi
}

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
    local time
    
    # Debug: Check if data file exists
    if [ ! -f "$data_file" ]; then
        echo -e "${YELLOW}WARNING: Data file does not exist: $data_file${NC}" >&2
        echo "0"
        return
    fi
    
    if [ "$engine" = "bright" ]; then
        # Create index (Bright uses query parameters)
        local response=$(curl -s -X POST "$url/indexes?id=$index_name&primaryKey=id")
        
        # Wait a moment for index to be fully initialized
        sleep 0.5
        
        # Index documents and capture response
        local start=$(date +%s%N)
        local upload_response=$(curl -s -X POST "$url/indexes/$index_name/documents?format=jsoneachrow" \
            -H 'Content-Type: application/json' \
            --data-binary @"$data_file")
        local end=$(date +%s%N)
        time=$(( (end - start) / 1000000 ))
    else
        # Meilisearch - async indexing, need to wait for task completion
        echo -e "${BLUE}Creating Meilisearch index: $index_name${NC}" >&2
        curl -s -X POST "$url/indexes" \
            -H "Content-Type: application/json" \
            -d '{"uid": "'"$index_name"'", "primaryKey": "id"}' > /dev/null
        
        # Convert JSON Lines to JSON array for Meilisearch and save to temp file
        local temp_file=$(mktemp)
        jq -s '.' < "$data_file" > "$temp_file"
        
        # Start timing
        local start=$(date +%s%N)
        
        # Submit documents and get task UID
        local task_response=$(curl -s -X POST "$url/indexes/$index_name/documents" \
            -H "Content-Type: application/json" \
            --data-binary @"$temp_file")
        
        local task_uid=$(echo "$task_response" | jq -r '.taskUid')
        echo -e "${BLUE}Meilisearch task UID: $task_uid${NC}" >&2
        
        # Poll task status until completed
        local task_status="enqueued"
        while [ "$task_status" != "succeeded" ] && [ "$task_status" != "failed" ]; do
            sleep 0.5  # Poll every 500ms
            local task_info=$(curl -s "$url/tasks/$task_uid")
            task_status=$(echo "$task_info" | jq -r '.status')
            echo -e "${BLUE}Task status: $task_status${NC}" >&2
        done
        
        local end=$(date +%s%N)
        time=$(( (end - start) / 1000000 ))
        
        if [ "$task_status" = "failed" ]; then
            echo -e "${YELLOW}Warning: Meilisearch indexing task failed${NC}" >&2
        fi
        
        rm -f "$temp_file"
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
    if curl -s -X POST "$BRIGHT_URL/indexes?id=test&primaryKey=id" > /dev/null 2>&1; then
        echo -e "${GREEN}Bright is ready${NC}"
        # Delete test index
        curl -s -X DELETE "$BRIGHT_URL/indexes/test" > /dev/null 2>&1
        break
    fi
    echo "Waiting for Bright to start..."
    sleep 1
done

# Arrays to store results by dataset size
declare -A BRIGHT_INDEX_TIMES
declare -A BRIGHT_SEARCH_TIMES
declare -A BRIGHT_MEMORY
declare -A MEILI_INDEX_TIMES
declare -A MEILI_SEARCH_TIMES
declare -A MEILI_MEMORY

# Run Bright benchmarks for different dataset sizes
echo -e "${GREEN}=== Benchmarking Bright ===${NC}"
for size in 1000 5000 10000; do
    echo -e "${YELLOW}Testing with $size documents...${NC}"
    
    index_name="products_$size"
    data_file="benchmarks/test_data_${size}.jsonl"
    
    # Test indexing
    echo -e "${YELLOW}Testing indexing for Bright...${NC}" >&2
    index_time=$(test_indexing "bright" "$BRIGHT_URL" "$index_name" "$data_file")
    BRIGHT_INDEX_TIMES[$size]=$index_time
    echo -e "Indexing time: ${GREEN}${index_time}ms${NC}"
    
    # Measure memory after indexing
    memory=$(get_memory_usage "$BRIGHT_PID")
    BRIGHT_MEMORY[$size]=$memory
    echo -e "Memory usage: ${GREEN}${memory}MB${NC}"
    
    sleep 2
    
    # Test search
    search1=$(test_search "bright" "$BRIGHT_URL" "$index_name" "laptop")
    search2=$(test_search "bright" "$BRIGHT_URL" "$index_name" "computer")
    search3=$(test_search "bright" "$BRIGHT_URL" "$index_name" "price:>100")
    avg_search=$(( (search1 + search2 + search3) / 3 ))
    BRIGHT_SEARCH_TIMES[$size]=$avg_search
    
    echo -e "Avg search time: ${GREEN}${avg_search}ms${NC}"
    
    # Cleanup index
    curl -s -X DELETE "$BRIGHT_URL/indexes/$index_name" > /dev/null 2>&1
    sleep 1
done

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

# Run Meilisearch benchmarks for different dataset sizes
echo -e "${GREEN}=== Benchmarking Meilisearch ===${NC}"
for size in 1000 5000 10000; do
    echo -e "${YELLOW}Testing with $size documents...${NC}"
    
    index_name="products_$size"
    data_file="benchmarks/test_data_${size}.jsonl"
    
    # Test indexing
    echo -e "${YELLOW}Testing indexing for Meilisearch...${NC}" >&2
    index_time=$(test_indexing "meilisearch" "$MEILI_URL" "$index_name" "$data_file")
    MEILI_INDEX_TIMES[$size]=$index_time
    echo -e "Indexing time: ${GREEN}${index_time}ms${NC}"
    
    # Measure memory after indexing
    memory=$(get_memory_usage "$MEILI_PID")
    MEILI_MEMORY[$size]=$memory
    echo -e "Memory usage: ${GREEN}${memory}MB${NC}"
    
    sleep 2
    
    # Test search
    search1=$(test_search "meilisearch" "$MEILI_URL" "$index_name" "laptop")
    search2=$(test_search "meilisearch" "$MEILI_URL" "$index_name" "computer")
    search3=$(test_search "meilisearch" "$MEILI_URL" "$index_name" "price")
    avg_search=$(( (search1 + search2 + search3) / 3 ))
    MEILI_SEARCH_TIMES[$size]=$avg_search
    
    echo -e "Avg search time: ${GREEN}${avg_search}ms${NC}"
    
    sleep 1
done

# Stop Meilisearch
kill $MEILI_PID

# Generate results
echo -e "${BLUE}=== Results ===${NC}"
RESULT_FILE="$RESULTS_DIR/benchmark_${TIMESTAMP}.json"

# Build JSON for all dataset sizes
cat > "$RESULT_FILE" <<EOF
{
  "timestamp": "$TIMESTAMP",
  "results": {
    "1000": {
      "bright": {
        "indexing_ms": ${BRIGHT_INDEX_TIMES[1000]:-0},
        "search_ms": ${BRIGHT_SEARCH_TIMES[1000]:-0},
        "memory_mb": ${BRIGHT_MEMORY[1000]:-0}
      },
      "meilisearch": {
        "indexing_ms": ${MEILI_INDEX_TIMES[1000]:-0},
        "search_ms": ${MEILI_SEARCH_TIMES[1000]:-0},
        "memory_mb": ${MEILI_MEMORY[1000]:-0}
      }
    },
    "5000": {
      "bright": {
        "indexing_ms": ${BRIGHT_INDEX_TIMES[5000]:-0},
        "search_ms": ${BRIGHT_SEARCH_TIMES[5000]:-0},
        "memory_mb": ${BRIGHT_MEMORY[5000]:-0}
      },
      "meilisearch": {
        "indexing_ms": ${MEILI_INDEX_TIMES[5000]:-0},
        "search_ms": ${MEILI_SEARCH_TIMES[5000]:-0},
        "memory_mb": ${MEILI_MEMORY[5000]:-0}
      }
    },
    "10000": {
      "bright": {
        "indexing_ms": ${BRIGHT_INDEX_TIMES[10000]:-0},
        "search_ms": ${BRIGHT_SEARCH_TIMES[10000]:-0},
        "memory_mb": ${BRIGHT_MEMORY[10000]:-0}
      },
      "meilisearch": {
        "indexing_ms": ${MEILI_INDEX_TIMES[10000]:-0},
        "search_ms": ${MEILI_SEARCH_TIMES[10000]:-0},
        "memory_mb": ${MEILI_MEMORY[10000]:-0}
      }
    }
  }
}
EOF

echo -e "${GREEN}Results saved to: $RESULT_FILE${NC}"

# Display comparison table
echo ""
echo -e "${BLUE}=== Performance Comparison ===${NC}"
printf "%-15s %-20s %-20s %-20s\n" "Dataset Size" "Bright Index (ms)" "Meili Index (ms)" "Winner"
printf "%-15s %-20s %-20s %-20s\n" "---------------" "--------------------" "--------------------" "--------------------"

for size in 1000 5000 10000; do
    bright_idx="${BRIGHT_INDEX_TIMES[$size]:-0}"
    meili_idx="${MEILI_INDEX_TIMES[$size]:-0}"
    
    if [ "$bright_idx" -lt "$meili_idx" ]; then
        diff=$(( (meili_idx - bright_idx) * 100 / meili_idx ))
        winner="Bright ($diff% faster)"
    else
        diff=$(( (bright_idx - meili_idx) * 100 / bright_idx ))
        winner="Meilisearch ($diff% faster)"
    fi
    
    printf "%-15s %-20s %-20s %-20s\n" "$size docs" "$bright_idx" "$meili_idx" "$winner"
done

echo ""
printf "%-15s %-20s %-20s %-20s\n" "Dataset Size" "Bright Search (ms)" "Meili Search (ms)" "Winner"
printf "%-15s %-20s %-20s %-20s\n" "---------------" "--------------------" "--------------------" "--------------------"

for size in 1000 5000 10000; do
    bright_search="${BRIGHT_SEARCH_TIMES[$size]:-0}"
    meili_search="${MEILI_SEARCH_TIMES[$size]:-0}"
    
    if [ "$bright_search" -lt "$meili_search" ]; then
        diff=$(( (meili_search - bright_search) * 100 / meili_search ))
        winner="Bright ($diff% faster)"
    else
        diff=$(( (bright_search - meili_search) * 100 / bright_search ))
        winner="Meilisearch ($diff% faster)"
    fi
    
    printf "%-15s %-20s %-20s %-20s\n" "$size docs" "$bright_search" "$meili_search" "$winner"
done

echo ""
printf "%-15s %-20s %-20s %-20s\n" "Dataset Size" "Bright Memory (MB)" "Meili Memory (MB)" "Winner"
printf "%-15s %-20s %-20s %-20s\n" "---------------" "--------------------" "--------------------" "--------------------"

for size in 1000 5000 10000; do
    bright_mem="${BRIGHT_MEMORY[$size]:-0}"
    meili_mem="${MEILI_MEMORY[$size]:-0}"
    
    # Ensure numeric values
    bright_mem=$(echo "$bright_mem" | grep -o '[0-9]*' | head -1)
    meili_mem=$(echo "$meili_mem" | grep -o '[0-9]*' | head -1)
    bright_mem=${bright_mem:-0}
    meili_mem=${meili_mem:-0}
    
    # Use arithmetic comparison
    if (( bright_mem < meili_mem )); then
        diff=$(( (meili_mem - bright_mem) * 100 / meili_mem ))
        winner="Bright ($diff% less)"
    elif (( meili_mem < bright_mem )); then
        diff=$(( (bright_mem - meili_mem) * 100 / bright_mem ))
        winner="Meilisearch ($diff% less)"
    else
        winner="Tie"
    fi
    
    printf "%-15s %-20s %-20s %-20s\n" "$size docs" "${bright_mem}MB" "${meili_mem}MB" "$winner"
done

echo ""
echo -e "${GREEN}Benchmark complete!${NC}"
