package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/bytedance/sonic"
)

type Product struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Price       float64  `json:"price"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	InStock     bool     `json:"inStock"`
}

var (
	categories = []string{"Electronics", "Clothing", "Books", "Home & Garden", "Sports", "Toys"}

	productNames = []string{
		"Laptop", "Computer", "Smartphone", "Tablet", "Headphones",
		"T-Shirt", "Jeans", "Sneakers", "Jacket", "Hat",
		"Novel", "Textbook", "Magazine", "Comic", "Dictionary",
		"Chair", "Table", "Lamp", "Plant", "Curtains",
		"Basketball", "Soccer Ball", "Tennis Racket", "Yoga Mat", "Dumbbell",
		"Action Figure", "Board Game", "Puzzle", "Doll", "LEGO Set",
	}

	adjectives = []string{
		"Premium", "Professional", "Deluxe", "Standard", "Basic",
		"Wireless", "Portable", "Compact", "Ergonomic", "Modern",
		"Classic", "Vintage", "Limited Edition", "Heavy-Duty", "Lightweight",
	}

	tags = []string{
		"sale", "new", "popular", "trending", "bestseller",
		"eco-friendly", "premium", "budget", "featured", "clearance",
	}
)

func randomString(arr []string) string {
	return arr[rand.Intn(len(arr))]
}

func randomTags() []string {
	count := rand.Intn(3) + 1
	result := make([]string, 0, count)
	used := make(map[string]bool)

	for i := 0; i < count; i++ {
		tag := randomString(tags)
		if !used[tag] {
			result = append(result, tag)
			used[tag] = true
		}
	}

	return result
}

func generateProduct(id int) Product {
	return Product{
		ID:          fmt.Sprintf("%d", id),
		Name:        fmt.Sprintf("%s %s", randomString(adjectives), randomString(productNames)),
		Description: fmt.Sprintf("High-quality %s for all your needs. Perfect for everyday use.", randomString(productNames)),
		Price:       float64(rand.Intn(500)+10) + rand.Float64(),
		Category:    randomString(categories),
		Tags:        randomTags(),
		InStock:     rand.Float32() > 0.2, // 80% in stock
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Create benchmarks directory
	if err := os.MkdirAll("benchmarks", 0755); err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		os.Exit(1)
	}

	// Generate different dataset sizes
	sizes := []int{1000, 5000, 10000}

	for _, size := range sizes {
		filename := fmt.Sprintf("benchmarks/test_data_%d.jsonl", size)
		file, err := os.Create(filename)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", filename, err)
			os.Exit(1)
		}

		fmt.Printf("Generating %d products to %s...\n", size, filename)

		for i := 1; i <= size; i++ {
			product := generateProduct(i)
			data, err := sonic.Marshal(product)
			if err != nil {
				fmt.Printf("Error marshaling product: %v\n", err)
				file.Close()
				os.Exit(1)
			}

			file.Write(data)
			file.WriteString("\n")
		}

		file.Close()
		fmt.Printf("✓ Generated %s\n", filename)
	}

	// Create a default test_data.jsonl symlink/copy
	defaultFile := "benchmarks/test_data.jsonl"
	if err := os.Remove(defaultFile); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: could not remove old default file: %v\n", err)
	}

	// Copy the 1000 item file as default
	input, err := os.ReadFile("benchmarks/test_data_1000.jsonl")
	if err != nil {
		fmt.Printf("Error reading source file: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(defaultFile, input, 0644); err != nil {
		fmt.Printf("Error creating default file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ All test data generated successfully!\n")
	fmt.Printf("  - test_data_1000.jsonl (1,000 documents)\n")
	fmt.Printf("  - test_data_5000.jsonl (5,000 documents)\n")
	fmt.Printf("  - test_data_10000.jsonl (10,000 documents)\n")
	fmt.Printf("  - test_data.jsonl (default, 1,000 documents)\n")
}
