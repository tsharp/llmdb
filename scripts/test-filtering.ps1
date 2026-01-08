# Test script for document filtering functionality

$baseUrl = "http://localhost:8080"
$db = "test_filters"
$table = "documents"

Write-Host "Testing Document Filtering Features" -ForegroundColor Green
Write-Host "=====================================" -ForegroundColor Green
Write-Host ""

# 1. Store documents with different tags and metadata
Write-Host "1. Storing documents with tags and metadata..." -ForegroundColor Yellow

$doc1 = @{
    content = "Introduction to Python programming for beginners"
    tags = @("python", "programming", "beginner")
    metadata = @{
        author = "Alice"
        category = "tutorial"
        difficulty = "easy"
        year = 2024
    }
} | ConvertTo-Json -Compress

$doc2 = @{
    content = "Advanced JavaScript patterns and best practices"
    tags = @("javascript", "programming", "advanced")
    metadata = @{
        author = "Bob"
        category = "tutorial"
        difficulty = "hard"
        year = 2024
    }
} | ConvertTo-Json -Compress

$doc3 = @{
    content = "Machine Learning with Python: A comprehensive guide"
    tags = @("python", "ml", "advanced")
    metadata = @{
        author = "Charlie"
        category = "guide"
        difficulty = "medium"
        year = 2024
    }
} | ConvertTo-Json -Compress

$doc4 = @{
    content = "Web development fundamentals using JavaScript"
    tags = @("javascript", "web", "beginner")
    metadata = @{
        author = "Alice"
        category = "tutorial"
        difficulty = "easy"
        year = 2023
    }
} | ConvertTo-Json -Compress

try {
    Invoke-RestMethod -Uri "$baseUrl/db/$db/$table" -Method Post -Body $doc1 -ContentType "application/json" | Out-Null
    Write-Host "  ✓ Stored document 1 (Python beginner tutorial)" -ForegroundColor Green
    
    Invoke-RestMethod -Uri "$baseUrl/db/$db/$table" -Method Post -Body $doc2 -ContentType "application/json" | Out-Null
    Write-Host "  ✓ Stored document 2 (JavaScript advanced tutorial)" -ForegroundColor Green
    
    Invoke-RestMethod -Uri "$baseUrl/db/$db/$table" -Method Post -Body $doc3 -ContentType "application/json" | Out-Null
    Write-Host "  ✓ Stored document 3 (Python ML guide)" -ForegroundColor Green
    
    Invoke-RestMethod -Uri "$baseUrl/db/$db/$table" -Method Post -Body $doc4 -ContentType "application/json" | Out-Null
    Write-Host "  ✓ Stored document 4 (JavaScript web beginner)" -ForegroundColor Green
} catch {
    Write-Host "  ✗ Error storing documents: $_" -ForegroundColor Red
    exit 1
}

Write-Host ""
Start-Sleep -Seconds 1

# 2. Search with tag filter - beginner only
Write-Host "2. Search for beginner content (tag filter)..." -ForegroundColor Yellow

$searchRequest = @{
    query = "programming"
    type = "fulltext"
    limit = 10
    filters = @{
        tag = "beginner"
    }
} | ConvertTo-Json -Compress

try {
    $results = Invoke-RestMethod -Uri "$baseUrl/db/$db/$table/search" -Method Post -Body $searchRequest -ContentType "application/json"
    Write-Host "  Found $($results.results.Count) beginner documents:" -ForegroundColor Green
    foreach ($result in $results.results) {
        Write-Host "    - $($result.document.content.Substring(0, [Math]::Min(50, $result.document.content.Length)))..." -ForegroundColor Cyan
        Write-Host "      Tags: $($result.document.tags -join ', ')" -ForegroundColor Gray
    }
} catch {
    Write-Host "  ✗ Search failed: $_" -ForegroundColor Red
}

Write-Host ""

# 3. Search with multiple tag filters
Write-Host "3. Search for Python AND advanced content (multiple tags)..." -ForegroundColor Yellow

$searchRequest = @{
    query = "guide"
    type = "fulltext"
    limit = 10
    filters = @{
        tags = @("python", "advanced")
    }
} | ConvertTo-Json -Compress

try {
    $results = Invoke-RestMethod -Uri "$baseUrl/db/$db/$table/search" -Method Post -Body $searchRequest -ContentType "application/json"
    Write-Host "  Found $($results.results.Count) Python + advanced documents:" -ForegroundColor Green
    foreach ($result in $results.results) {
        Write-Host "    - $($result.document.content.Substring(0, [Math]::Min(50, $result.document.content.Length)))..." -ForegroundColor Cyan
        Write-Host "      Tags: $($result.document.tags -join ', ')" -ForegroundColor Gray
    }
} catch {
    Write-Host "  ✗ Search failed: $_" -ForegroundColor Red
}

Write-Host ""

# 4. Search with metadata filter - by author
Write-Host "4. Search for documents by Alice (metadata filter)..." -ForegroundColor Yellow

$searchRequest = @{
    query = "tutorial"
    type = "fulltext"
    limit = 10
    filters = @{
        author = "Alice"
    }
} | ConvertTo-Json -Compress

try {
    $results = Invoke-RestMethod -Uri "$baseUrl/db/$db/$table/search" -Method Post -Body $searchRequest -ContentType "application/json"
    Write-Host "  Found $($results.results.Count) documents by Alice:" -ForegroundColor Green
    foreach ($result in $results.results) {
        Write-Host "    - $($result.document.content.Substring(0, [Math]::Min(50, $result.document.content.Length)))..." -ForegroundColor Cyan
        Write-Host "      Author: $($result.document.metadata.author), Year: $($result.document.metadata.year)" -ForegroundColor Gray
    }
} catch {
    Write-Host "  ✗ Search failed: $_" -ForegroundColor Red
}

Write-Host ""

# 5. Search with combined filters - tags + metadata
Write-Host "5. Search for JavaScript tutorials from 2024 (combined filters)..." -ForegroundColor Yellow

$searchRequest = @{
    query = "development"
    type = "fulltext"
    limit = 10
    filters = @{
        tag = "javascript"
        category = "tutorial"
        year = 2024
    }
} | ConvertTo-Json -Compress

try {
    $results = Invoke-RestMethod -Uri "$baseUrl/db/$db/$table/search" -Method Post -Body $searchRequest -ContentType "application/json"
    Write-Host "  Found $($results.results.Count) JavaScript tutorials from 2024:" -ForegroundColor Green
    foreach ($result in $results.results) {
        Write-Host "    - $($result.document.content.Substring(0, [Math]::Min(50, $result.document.content.Length)))..." -ForegroundColor Cyan
        Write-Host "      Tags: $($result.document.tags -join ', '), Category: $($result.document.metadata.category)" -ForegroundColor Gray
    }
} catch {
    Write-Host "  ✗ Search failed: $_" -ForegroundColor Red
}

Write-Host ""

# 6. Search with difficulty filter
Write-Host "6. Search for easy difficulty content (metadata filter)..." -ForegroundColor Yellow

$searchRequest = @{
    query = "tutorial"
    type = "fulltext"
    limit = 10
    filters = @{
        difficulty = "easy"
    }
} | ConvertTo-Json -Compress

try {
    $results = Invoke-RestMethod -Uri "$baseUrl/db/$db/$table/search" -Method Post -Body $searchRequest -ContentType "application/json"
    Write-Host "  Found $($results.results.Count) easy difficulty documents:" -ForegroundColor Green
    foreach ($result in $results.results) {
        Write-Host "    - $($result.document.content.Substring(0, [Math]::Min(50, $result.document.content.Length)))..." -ForegroundColor Cyan
        Write-Host "      Difficulty: $($result.document.metadata.difficulty)" -ForegroundColor Gray
    }
} catch {
    Write-Host "  ✗ Search failed: $_" -ForegroundColor Red
}

Write-Host ""
Write-Host "=====================================" -ForegroundColor Green
Write-Host "Testing Complete!" -ForegroundColor Green
Write-Host ""
Write-Host "Note: Make sure the LLMDB server is running on $baseUrl before running this script." -ForegroundColor Yellow
