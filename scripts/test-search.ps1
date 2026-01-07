# Test search functionality for Context Pipeline API

$baseUrl = "http://192.168.91.57:8080"
$dbId = "products"

# Full-text search
Write-Host "=== Full-Text Search ===" -ForegroundColor Cyan
$searchRequest = @{
    query = "Product"
    type = "fulltext"
    limit = 100
} | ConvertTo-Json

$response = Invoke-RestMethod -Uri "$baseUrl/db/$dbId/search" `
    -Method POST `
    -ContentType "application/json" `
    -Body $searchRequest

Write-Host "Found $($response.total) results:" -ForegroundColor Green
$response.results | ForEach-Object {
    Write-Host "  - Document ID: $($_.document.id)"
    Write-Host "    Score: $("{0:F6}" -f $_.score)"
    Write-Host ""
}

# Vector search (will fail until embedder is implemented)
Write-Host "`n=== Vector Search ===" -ForegroundColor Cyan
$vectorSearchRequest = @{
    query = "Product information"
    type = "vector"
    limit = 100
} | ConvertTo-Json

try {
    $vectorResponse = Invoke-RestMethod -Uri "$baseUrl/db/$dbId/search" `
        -Method POST `
        -ContentType "application/json" `
        -Body $vectorSearchRequest
    
    Write-Host "Found $($vectorResponse.total) results" -ForegroundColor Green
    $vectorResponse.results | ForEach-Object {
      Write-Host "  - Document ID: $($_.document.id)"
      Write-Host "    Score: $("{0:F6}" -f $_.score)"
      Write-Host ""
    }
} catch {
    Write-Host "Vector search not yet implemented (expected)" -ForegroundColor Yellow
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Yellow
}

Write-Host "`n=== Search Complete ===" -ForegroundColor Cyan
