param(
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$')]
    [string]$Version = '3.0.0',
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$projectRoot = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $projectRoot 'dist'
$stage = Join-Path $dist 'app'
$runtimeTemp = Join-Path $dist 'temurin-download'
$javaArchive = Join-Path $runtimeTemp 'temurin-jre-25.zip'
$runtimeExtract = Join-Path $runtimeTemp 'expanded'
$portableArchive = Join-Path $dist "Pingu-Portable-$Version-Windows-x64.zip"
$installerExecutable = Join-Path $dist "Pingu-Setup-$Version-Windows-x64.exe"
$env:PINGU_VERSION = $Version
$env:CGO_ENABLED = '1'
$env:CC = 'gcc'
$staticLinkerFlags = "-H=windowsgui -s -w -extldflags '-static'"
$temurinVersion = '25.0.4.1+1'
$temurinArchiveName = 'OpenJDK25U-jre_x64_windows_hotspot_25.0.4.1_1.zip'
$temurinUrl = "https://github.com/adoptium/temurin25-binaries/releases/download/jdk-25.0.4.1%2B1/$temurinArchiveName"
$temurinSHA256 = '4c95451cea98556def2c54f7782933f52a26d4a36bd85e1d59f0364464828b07'

foreach ($command in @('go.exe', 'gcc.exe', 'objdump.exe')) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "$command não foi encontrado no PATH. Instale o Go e o MinGW-w64 antes de compilar."
    }
}

if (Test-Path $dist) {
    Remove-Item -LiteralPath $dist -Recurse -Force
}
New-Item -ItemType Directory -Path $stage -Force | Out-Null
New-Item -ItemType Directory -Path $runtimeTemp -Force | Out-Null

Push-Location $projectRoot
try {
    go mod download
    if ($LASTEXITCODE -ne 0) {
        throw "go mod download retornou o código $LASTEXITCODE."
    }
    $goFiles = Get-ChildItem -LiteralPath $projectRoot -Filter '*.go' -File -Recurse |
        Where-Object { $_.FullName -notmatch '[\\/]vendor[\\/]' } |
        Select-Object -ExpandProperty FullName
    gofmt -w $goFiles
    if ($LASTEXITCODE -ne 0) {
        throw "gofmt retornou o código $LASTEXITCODE."
    }
    if (-not $SkipTests) {
        go test -buildvcs=false -tags ci ./...
        if ($LASTEXITCODE -ne 0) { throw "go test retornou o código $LASTEXITCODE." }
        go vet -buildvcs=false -tags ci ./...
        if ($LASTEXITCODE -ne 0) { throw "go vet retornou o código $LASTEXITCODE." }
        go test -race -buildvcs=false ./internal/commands ./internal/manager ./internal/proxy ./internal/api ./internal/i18n
        if ($LASTEXITCODE -ne 0) { throw "go test -race retornou o código $LASTEXITCODE." }
    }

    $executable = Join-Path $stage 'pingu.exe'
    $iconFile = Join-Path $projectRoot 'assets\icon.ico'
    if (-not (Test-Path -LiteralPath $iconFile)) {
        throw "Missing Windows icon: $iconFile"
    }
    Write-Host 'Embedding the Pingu icon in the Windows executable...'
    go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico $iconFile -o (Join-Path $projectRoot 'rsrc_windows_amd64.syso')
    if ($LASTEXITCODE -ne 0) {
        throw "Icon resource generation failed: $LASTEXITCODE"
    }
    $linkerFlags = "$staticLinkerFlags -X main.appVersion=$Version"
    Write-Host "Compilando pingu.exe com CGO_ENABLED=1, CC=gcc e vínculo estático do runtime MinGW..."
    go build -buildvcs=false -trimpath -ldflags $linkerFlags -o $executable .
    if ($LASTEXITCODE -ne 0) {
        throw "go build retornou o código $LASTEXITCODE."
    }

    $objdump = Get-Command objdump.exe -ErrorAction SilentlyContinue
    if (-not $objdump) {
        throw 'objdump.exe não foi encontrado. Confirme que o MinGW-w64 está instalado e disponível no PATH.'
    }
    $imports = (& $objdump.Source -p $executable) -join "`n"
    if ($LASTEXITCODE -ne 0) {
        throw "objdump.exe retornou o código $LASTEXITCODE ao verificar pingu.exe."
    }
    $forbiddenImports = @(
        'libmcfgthread-2.dll',
        'libgcc_s_seh-1.dll',
        'libstdc++-6.dll',
        'libwinpthread-1.dll',
        'libssp-0.dll',
        'libgomp-1.dll',
        'libquadmath-0.dll'
    )
    foreach ($library in $forbiddenImports) {
        if ($imports -match [regex]::Escape($library)) {
            throw "O executável ainda depende de $library. Verifique o toolchain MinGW-w64 e as flags de vínculo estático."
        }
    }
    Write-Host 'Verificação concluída: nenhuma DLL de runtime do MinGW foi importada por pingu.exe.'

    Write-Host "Baixando Eclipse Temurin JRE $temurinVersion..."
    Invoke-WebRequest -Uri $temurinUrl -OutFile $javaArchive -MaximumRedirection 10 -UseBasicParsing
    $actualJavaSHA256 = (Get-FileHash -LiteralPath $javaArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualJavaSHA256 -ne $temurinSHA256) {
        throw "SHA-256 do runtime Java inválido. Esperado $temurinSHA256; obtido $actualJavaSHA256."
    }
    Write-Host "SHA-256 do runtime Java validado: $actualJavaSHA256"
    Expand-Archive -LiteralPath $javaArchive -DestinationPath $runtimeExtract -Force
    $runtimeRoot = Get-ChildItem -LiteralPath $runtimeExtract -Directory | Select-Object -First 1
    if (-not $runtimeRoot -or -not (Test-Path (Join-Path $runtimeRoot.FullName 'bin\java.exe'))) {
        throw 'O pacote Temurin baixado não contém bin\java.exe.'
    }
    Copy-Item -LiteralPath $runtimeRoot.FullName -Destination (Join-Path $stage 'runtime') -Recurse -Force
    Copy-Item -LiteralPath (Join-Path $projectRoot 'README.md') -Destination $stage -Force
    Copy-Item -LiteralPath (Join-Path $projectRoot 'README.pt-BR.md') -Destination $stage -Force
    Copy-Item -LiteralPath (Join-Path $projectRoot 'LICENSE') -Destination $stage -Force
    Remove-Item -LiteralPath $runtimeTemp -Recurse -Force

    Write-Host 'Criando a distribuição portátil...'
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $portableArchive -CompressionLevel Optimal -Force

    $isccCandidates = @(
        (Get-Command ISCC.exe -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source -ErrorAction SilentlyContinue),
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
    ) | Where-Object { $_ -and (Test-Path $_) }
    $iscc = $isccCandidates | Select-Object -First 1
    if (-not $iscc) {
        throw 'Inno Setup 6 não encontrado. Instale em https://jrsoftware.org/isdl.php.'
    }
    & $iscc (Join-Path $projectRoot 'installer\pingu.iss')
    if ($LASTEXITCODE -ne 0) {
        throw "O Inno Setup retornou o código $LASTEXITCODE."
    }

    foreach ($artifact in @($installerExecutable, $portableArchive)) {
        if (-not (Test-Path -LiteralPath $artifact -PathType Leaf)) {
            throw "O artefato esperado não foi criado: $artifact"
        }
    }

    Get-Item -LiteralPath $installerExecutable, $portableArchive |
        Get-FileHash -Algorithm SHA256 |
        ForEach-Object { '{0}  {1}' -f $_.Hash.ToLowerInvariant(), (Split-Path $_.Path -Leaf) } |
        Set-Content -LiteralPath (Join-Path $dist 'SHA256SUMS.txt') -Encoding ascii
}
finally {
    Pop-Location
}

Write-Host "Pingu $Version pronto."
Write-Host "Instalador: $installerExecutable"
Write-Host "Portátil:   $portableArchive"
Write-Host "Checksums:  $(Join-Path $dist 'SHA256SUMS.txt')"
