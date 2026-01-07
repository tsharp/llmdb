#!/bin/bash

# Test script for dynamic table API

BASE_URL="http://localhost:8080"

echo "=== Testing Dynamic Table API ==="
echo ""

# Test 1: Create a document in mydb/users table
echo "1. Creating document in mydb/users table..."
curl -X POST "$BASE_URL/db/mydb/users" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "user1",
    "content": "John Doe is a software engineer",
    "metadata": {"role": "engineer", "department": "IT"}
  }'
echo -e "\n"

# Test 2: Create a document in mydb/products table
echo "2. Creating document in mydb/products table..."
curl -X POST "$BASE_URL/db/mydb/products" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "prod1",
    "content": "Laptop computer with 16GB RAM",
    "metadata": {"price": 999, "category": "electronics"}
  }'
echo -e "\n"

# Test 3: Create a document in another database
echo "3. Creating document in analytics/events table..."
curl -X POST "$BASE_URL/db/analytics/events" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "event1",
    "content": "User login event from mobile app",
    "metadata": {"timestamp": "2026-01-07T10:00:00Z", "platform": "mobile"}
  }'
echo -e "\n"

# Test 4: List all databases
echo "4. Listing all databases..."
curl -s "$BASE_URL/db" | jq
echo ""

# Test 5: List tables in mydb
echo "5. Listing tables in mydb..."
curl -s "$BASE_URL/db/mydb" | jq
echo ""

# Test 6: List documents in mydb/users
echo "6. Listing documents in mydb/users..."
curl -s "$BASE_URL/db/mydb/users" | jq
echo ""

# Test 7: Get a specific document
echo "7. Getting specific document from mydb/users..."
curl -s "$BASE_URL/db/mydb/users/user1" | jq
echo ""

# Test 8: Search in mydb/users
echo "8. Searching in mydb/users..."
curl -X POST "$BASE_URL/db/mydb/users/search" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "engineer",
    "type": "fulltext",
    "limit": 10
  }' | jq
echo ""

echo "=== Tests Complete ==="
