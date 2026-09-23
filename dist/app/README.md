# Pingu — Minecraft Server Manager

[Português (Brasil)](README.pt-BR.md)

Pingu is a native Windows desktop application for creating and managing Minecraft Java + Bedrock servers with PaperMC, Geyser, and Floodgate. The interface is built with Go and Fyne v2 and runs without opening a terminal window.

## Download and install

Users do not need to install Go, GCC, or configure Java manually.

1. Open the [latest Release](../../releases/latest).
2. Download `Pingu-Setup-VERSION-Windows-x64.exe` for the guided installation, or `Pingu-Portable-VERSION-Windows-x64.zip` for the portable package.
3. Run the installer and choose the language, installation directory, and shortcuts.
4. Select **Launch Pingu** when installation finishes.

The installer contains the statically linked `pingu.exe`, a 64-bit Eclipse Temurin Java 25 LTS runtime, Start Menu shortcuts, a Desktop shortcut enabled by default, and a standard Windows uninstaller. Administrator privileges are required because the application is installed under 64-bit Program Files.

The first time **PLAY** is selected, Pingu downloads the latest compatible PaperMC, Geyser, Floodgate, Chunky, and CoreProtect versions. This step requires an internet connection.

### Microsoft Defender SmartScreen

The installer is not digitally signed, so Microsoft Defender SmartScreen may display a warning. If the file came from this repository's official Releases page and its SHA-256 matches `SHA256SUMS.txt`:

1. Select **More info**.
2. Confirm that the file name and source are correct.
3. Select **Run anyway**.

Do not bypass SmartScreen for a file obtained from an unknown source.

## Java requirement

Pingu requires Java **25 or newer** to start PaperMC.

- The installer includes Java 25 LTS and Pingu checks `./runtime/bin/java.exe` first.
- If the bundled runtime is missing, Pingu searches for Java in the system `PATH`.
- Java 24 or older is rejected with a clear message in the interface.
- Later versions, such as Java 26, are accepted.

## Main features

- Five tabs: Home, Console, Players, Server Controls, and Settings/Files.
- Separate Java and Bedrock cards with address, supported version, and live ON/OFF status.
- One-click copying of each `IP:port` address.
- Java TCP and Bedrock UDP proxies listening on `0.0.0.0`.
- Automatic free-port detection starting at `25565/TCP` and `19132/UDP`.
- PaperMC startup on the first incoming packet.
- Graceful shutdown after 15 minutes without players or traffic.
- Immediate proxy shutdown through context cancellation and socket closure.
- Asynchronous dependency downloads with progress and cancellation.
- Console commands, OP, kick, temporary ban, and pardon actions.
- Live English and Brazilian Portuguese switching.
- Circular console buffer limited to 2,500 entries.
- Persistent settings in `server/config.json`.

## Repository structure

```text
Pingu-Minecraft-Server-Manager/
├── .github/workflows/release.yml
├── installer/pingu.iss
├── scripts/build-release.ps1
├── internal/
│   ├── api/
│   ├── gui/
│   ├── i18n/
│   ├── manager/
│   └── proxy/
├── FyneApp.toml
├── go.mod
├── go.sum
├── LICENSE
├── main.go
├── README.md
└── README.pt-BR.md
```

## Build on Windows

Developer requirements:

- 64-bit Windows 10 or 11;
- Go 1.23 or newer;
- GCC/MinGW-w64 for Fyne's CGO build;
- Inno Setup 6;
- PowerShell 5.1 or newer;
- internet access to download Eclipse Temurin Java 25.

Install the build tools from an Administrator PowerShell terminal:

```powershell
choco install golang mingw innosetup -y
```

Build the application, installer, and portable package:

```powershell
.\scripts\build-release.ps1 -Version 3.0.0
```

The release files are written to `dist/`:

```text
dist/
├── Pingu-Setup-3.0.0-Windows-x64.exe
├── Pingu-Portable-3.0.0-Windows-x64.zip
└── SHA256SUMS.txt
```

To build only the executable during development:

```powershell
$env:CGO_ENABLED = "1"
go build -buildvcs=false -trimpath -ldflags="-H=windowsgui -s -w -extldflags '-static'" -o pingu.exe .
```

The release script and workflow also inspect the executable with `objdump.exe`. The build fails if `pingu.exe` imports `libmcfgthread-2.dll`, `libgcc_s_seh-1.dll`, `libstdc++-6.dll`, or `libwinpthread-1.dll`.

## Tests

```powershell
go mod download
go test -buildvcs=false -tags ci ./...
go vet -buildvcs=false -tags ci ./...
go test -race -buildvcs=false ./internal/manager ./internal/proxy ./internal/api ./internal/i18n
```

## User data and networking

The default installation directory is `%ProgramFiles%\Pingu Minecraft Server Manager`. Writable server data is stored separately in `%LOCALAPPDATA%\Pingu Minecraft Server Manager\server`, which contains worlds, settings, logs, and plugins. The uninstaller preserves these server files to reduce the risk of accidental world loss; back them up before deleting them manually.

For LAN players, share the addresses shown on the Home tab. For internet access, forward the displayed Java TCP and Bedrock UDP ports on the router and allow `pingu.exe` and the bundled Java runtime through Windows Firewall. Pingu does not use Playit.gg or another third-party tunnel.

## License

Released under the [MIT License](LICENSE).

## Official documentation

- [Fyne](https://docs.fyne.io/)
- [PaperMC](https://docs.papermc.io/)
- [GeyserMC](https://geysermc.org/wiki/)
- [Eclipse Temurin](https://adoptium.net/temurin/releases/)
- [Inno Setup](https://jrsoftware.org/isinfo.php)
