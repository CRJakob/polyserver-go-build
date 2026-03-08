#!/usr/bin/env pwsh
<#
.SYNOPSIS
Build Polyserver for all Go-supported platforms locally.
Outputs binaries to ./out/ folder.

.EXAMPLE
.\build-all-platforms.ps1
#>

param(
    [switch]$PriorityOnly = $false,
    [switch]$Verbose = $false
)

# Colors for output
$colors = @{
    Success = 'Green'
    Warning = 'Yellow'
    Error   = 'Red'
    Info    = 'Cyan'
}

function Write-Status {
    param([string]$Message, [string]$Status = 'Info')
    $color = $colors[$Status]
    Write-Host $Message -ForegroundColor $color
}

# Create output directory
if (-not (Test-Path 'out')) {
    New-Item -ItemType Directory -Name 'out' -Force | Out-Null
    Write-Status "Created ./out/ directory" -Status Info
}

# Check if Go is installed
try {
    $goVersion = go version 2>$null
    Write-Status "Using: $goVersion" -Status Info
} catch {
    Write-Status "Go is not installed or not in PATH" -Status Error
    exit 1
}

# Priority targets (built first)
$priorityTargets = @(
    @{ OS = 'windows'; Arch = 'amd64' },
    @{ OS = 'linux'; Arch = 'amd64' },
    @{ OS = 'linux'; Arch = 'arm64' },
    @{ OS = 'darwin'; Arch = 'amd64' },
    @{ OS = 'darwin'; Arch = 'arm64' }
)

# Get all supported targets from Go
$allTargets = @()
try {
    $output = & go tool dist list
    foreach ($line in $output) {
        $parts = $line -split '/'
        if ($parts.Count -eq 2) {
            $allTargets += @{ OS = $parts[0]; Arch = $parts[1] }
        }
    }
    Write-Status "Found $($allTargets.Count) supported platform/arch combinations" -Status Info
} catch {
    Write-Status "Failed to get platform list from Go" -Status Error
    exit 1
}

# Track results
$successful = @()
$failed = @()
$skipped = @()
$startTime = Get-Date

# Build priority targets first
Write-Status "`n=== Building Priority Targets ===" -Status Info
foreach ($target in $priorityTargets) {
    $goos = $target.OS
    $goarch = $target.Arch
    $output = "out/polyserver_${goos}_${goarch}"
    
    if ($goos -eq 'windows') {
        $output += '.exe'
    }
    
    Write-Host "Building $goos/$goarch -> $output... " -NoNewline
    
    try {
        $env:GOOS = $goos
        $env:GOARCH = $goarch
        
        & go build -o $output . 2>$null
        
        if ($LASTEXITCODE -eq 0) {
            $size = (Get-Item $output).Length / 1MB
            Write-Status "✓ ($([math]::Round($size, 2)) MB)" -Status Success
            $successful += "$goos/$goarch"
        } else {
            Write-Status "✗ (build failed)" -Status Error
            $failed += "$goos/$goarch"
        }
    } catch {
        Write-Status "✗ (exception: $_)" -Status Error
        $failed += "$goos/$goarch"
    }
}

# If not priority-only, build remaining targets
if (-not $PriorityOnly) {
    Write-Status "`n=== Building Remaining Targets ===" -Status Info
    
    $prioritySet = @{}
    foreach ($target in $priorityTargets) {
        $prioritySet["$($target.OS)/$($target.Arch)"] = $true
    }
    
    foreach ($target in $allTargets) {
        $targetName = "$($target.OS)/$($target.Arch)"
        
        # Skip priority targets (already built)
        if ($prioritySet[$targetName]) {
            continue
        }
        
        $goos = $target.OS
        $goarch = $target.Arch
        $output = "out/polyserver_${goos}_${goarch}"
        
        if ($goos -eq 'windows') {
            $output += '.exe'
        }
        
        Write-Host "Building $targetName -> $output... " -NoNewline
        
        try {
            $env:GOOS = $goos
            $env:GOARCH = $goarch
            
            # Suppress stderr for exotic platforms that may not be fully supported
            & go build -o $output . 2>$null
            
            if ($LASTEXITCODE -eq 0) {
                $size = (Get-Item $output).Length / 1MB
                Write-Status "✓ ($([math]::Round($size, 2)) MB)" -Status Success
                $successful += $targetName
            } else {
                if ($Verbose) {
                    Write-Status "✗ (build failed - may be unsupported)" -Status Warning
                }
                $failed += $targetName
            }
        } catch {
            if ($Verbose) {
                Write-Status "✗ (exception: $_)" -Status Warning
            }
            $failed += $targetName
        }
    }
}

# Cleanup environment variables
Remove-Item env:GOOS -ErrorAction SilentlyContinue
Remove-Item env:GOARCH -ErrorAction SilentlyContinue

# Summary
$duration = (Get-Date) - $startTime
Write-Status "`n=== Build Summary ===" -Status Info

$totalSize = (Get-ChildItem out -Recurse -File | Measure-Object -Property Length -Sum).Sum / 1MB
Write-Host "Total build time: $([math]::Round($duration.TotalSeconds, 1))s"
Write-Host "Total artifacts: $($successful.Count)"
Write-Host "Total size: $([math]::Round($totalSize, 2)) MB"

if ($successful.Count -gt 0) {
    Write-Status "`n✓ Successful builds ($($successful.Count)):" -Status Success
    $successful | Sort-Object | ForEach-Object { Write-Host "  - $_" }
}

if ($failed.Count -gt 0) {
    Write-Status "`n✗ Failed builds ($($failed.Count)):" -Status Warning
    $failed | Sort-Object | ForEach-Object { Write-Host "  - $_" }
    Write-Host "(These platforms may be unsupported or require cgo)"
}

Write-Status "`nArtifacts saved to: ./out/" -Status Info
Write-Host "Run 'ls out/' to list all binaries"

# Exit with error if any priority targets failed
$failedPriorities = $failed | Where-Object { 
    $_ -in @('windows/amd64', 'linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64') 
}

if ($failedPriorities) {
    Write-Status "`n⚠️  Warning: One or more priority targets failed!" -Status Error
    exit 1
}

exit 0