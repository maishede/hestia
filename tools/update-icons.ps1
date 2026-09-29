# Regenerate all application icons from icon-master.png on Windows.
# Requires PowerShell, System.Drawing and GNU windres (MinGW).
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$root = Split-Path -Parent $PSScriptRoot
$windres = Get-Command windres -ErrorAction Stop
$master = [System.Drawing.Bitmap]::FromFile((Join-Path $root 'icon-master.png'))
$frames = @()

try {
    if ($master.Width -ne $master.Height) {
        throw 'icon-master.png must be square.'
    }

    foreach ($size in @(16, 24, 32, 48, 64, 128, 256)) {
        $bitmap = [System.Drawing.Bitmap]::new($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
        $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
        $stream = [System.IO.MemoryStream]::new()
        try {
            $graphics.CompositingMode = [System.Drawing.Drawing2D.CompositingMode]::SourceCopy
            $graphics.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
            $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
            $graphics.DrawImage($master, [System.Drawing.Rectangle]::new(0, 0, $size, $size))
            $bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
            $frames += [pscustomobject]@{ Size = $size; Data = $stream.ToArray() }
        } finally {
            $stream.Dispose()
            $graphics.Dispose()
            $bitmap.Dispose()
        }
    }
} finally {
    $master.Dispose()
}

foreach ($asset in @(
    @{ Path = 'icon-32.png'; Size = 32 },
    @{ Path = 'icon-256.png'; Size = 256 },
    @{ Path = 'web/assets/img/favicon.png'; Size = 64 },
    @{ Path = 'web/assets/img/logo.png'; Size = 256 },
    @{ Path = 'web/gui/img/logo.png'; Size = 256 }
)) {
    $frame = $frames | Where-Object Size -EQ $asset.Size
    [System.IO.File]::WriteAllBytes((Join-Path $root $asset.Path), $frame.Data)
}

# ICO directory followed by PNG frames (supported by Windows Vista and newer).
$icoStream = [System.IO.MemoryStream]::new()
$writer = [System.IO.BinaryWriter]::new($icoStream)
try {
    $writer.Write([uint16]0)
    $writer.Write([uint16]1)
    $writer.Write([uint16]$frames.Count)
    $offset = 6 + 16 * $frames.Count
    foreach ($frame in $frames) {
        $dimension = if ($frame.Size -eq 256) { 0 } else { $frame.Size }
        $writer.Write([byte]$dimension)
        $writer.Write([byte]$dimension)
        $writer.Write([byte]0)
        $writer.Write([byte]0)
        $writer.Write([uint16]1)
        $writer.Write([uint16]32)
        $writer.Write([uint32]$frame.Data.Length)
        $writer.Write([uint32]$offset)
        $offset += $frame.Data.Length
    }
    foreach ($frame in $frames) { $writer.Write([byte[]]$frame.Data) }
    $writer.Flush()
    [System.IO.File]::WriteAllBytes((Join-Path $root 'icon.ico'), $icoStream.ToArray())
} finally {
    $writer.Dispose()
    $icoStream.Dispose()
}

# Go embeds this checked-in resource directly; normal builds do not need windres.
$rcPath = [System.IO.Path]::GetTempFileName()
$resourcePath = [System.IO.Path]::GetTempFileName()
try {
    $icoPath = (Join-Path $root 'icon.ico').Replace('\', '/')
    [System.IO.File]::WriteAllText($rcPath, "LANGUAGE 9, 1`n1 ICON ""$icoPath""`n")
    & $windres.Source --target=pe-x86-64 --input-format=rc --output-format=coff --input $rcPath --output $resourcePath
    if ($LASTEXITCODE -ne 0) { throw 'windres failed to compile the Windows icon resource.' }
    Copy-Item -LiteralPath $resourcePath -Destination (Join-Path $root 'rsrc_windows_amd64.syso') -Force
} finally {
    Remove-Item -LiteralPath $rcPath, $resourcePath -Force
}

Write-Host 'Updated PNGs, seven ICO sizes (16-256 px), and the Windows amd64 resource.'
